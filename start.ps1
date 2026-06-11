# =============================================================================
# 古罗马水道工程结构健康与现代修复评估系统
# Windows PowerShell 启动脚本
# =============================================================================

$ErrorActionPreference = "Continue"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

Write-Host "╔═══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  古罗马水道工程结构健康与现代修复评估系统                       ║" -ForegroundColor Cyan
Write-Host "║  Aqueduct Structural Health & Rehabilitation Decision System ║" -ForegroundColor Cyan
Write-Host "╚═══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

function Step {
    param([string]$msg)
    Write-Host "`n[$(Get-Date -Format 'HH:mm:ss')] " -ForegroundColor Gray -NoNewline
    Write-Host $msg -ForegroundColor Yellow
}

function Success { param([string]$msg) Write-Host "  ✓ " -ForegroundColor Green -NoNewline; Write-Host $msg }
function Warn { param([string]$msg) Write-Host "  ⚠ " -ForegroundColor Yellow -NoNewline; Write-Host $msg }
function Err { param([string]$msg) Write-Host "  ✗ " -ForegroundColor Red -NoNewline; Write-Host $msg }

# ---------------------------------------------------------------------------
Step "Step 1/6: 检查 Docker 服务状态"
try {
    $dockerOk = docker version --format '{{.Server.Version}}' 2>$null
    if ($dockerOk) {
        Success "Docker 运行中 (Server v$dockerOk)"
    } else {
        Err "Docker 未运行，请先启动 Docker Desktop"
        exit 1
    }
} catch {
    Err "Docker 不可用: $_"
    exit 1
}

# ---------------------------------------------------------------------------
Step "Step 2/6: 启动 TimescaleDB 和 MQTT 服务容器"
docker compose up -d --wait

if ($LASTEXITCODE -ne 0) {
    Warn "Docker Compose 返回非零，继续尝试..."
}

Start-Sleep -Seconds 3

$tsdbOk = $false
for ($i = 0; $i -lt 15; $i++) {
    $result = docker exec aqueduct-timescaledb pg_isready -U postgres -d aqueduct_monitor 2>$null
    if ($LASTEXITCODE -eq 0 -or $result -match "accepting") {
        $tsdbOk = $true
        break
    }
    Write-Host "  等待数据库就绪... ($i/15)" -ForegroundColor Gray
    Start-Sleep -Seconds 2
}

if ($tsdbOk) {
    Success "TimescaleDB 已就绪 (aqueduct_monitor)"
} else {
    Warn "TimescaleDB 可能仍在初始化，请稍候..."
}

# ---------------------------------------------------------------------------
Step "Step 2b/6: 执行 Feature SQL 初始化"
try {
    $featureSqlPath = Join-Path $ScriptDir "database\features.sql"
    if (Test-Path $featureSqlPath) {
        docker exec -i aqueduct-timescaledb psql -U postgres -d aqueduct_monitor -c "SELECT 1" 2>$null
        $sqlContent = Get-Content $featureSqlPath -Raw
        $sqlContent | docker exec -i aqueduct-timescaledb psql -U postgres -d aqueduct_monitor 2>$null | Out-Null
        Success "Feature SQL 执行完成 (ON CONFLICT DO NOTHING)"
    } else {
        Warn "Feature SQL 文件不存在: $featureSqlPath"
    }
} catch {
    Warn "Feature SQL 执行警告 (不影响运行，将使用内存默认数据): $_"
}

# ---------------------------------------------------------------------------
Step "Step 3/6: 安装 Go 后端依赖"
Set-Location backend
try {
    go mod download 2>&1 | Out-Null
    go mod tidy 2>&1 | Out-Null
    Success "Go 模块就绪"
} catch {
    Warn "Go 依赖安装警告: $_"
}
Set-Location $ScriptDir

# ---------------------------------------------------------------------------
Step "Step 4/6: 安装模拟器依赖"
Set-Location simulator
try {
    go mod tidy 2>&1 | Out-Null
    Success "模拟器依赖就绪"
} catch {
    Warn "模拟器依赖警告: $_"
}
Set-Location $ScriptDir

# ---------------------------------------------------------------------------
Step "Step 5/6: 启动 Go 后端服务 (端口 8080)"
$backendProc = Start-Process powershell -ArgumentList @"
    -NoExit, -Command, 
    `$Host.UI.RawUI.WindowTitle='后端服务 Backend - localhost:8080';
    Set-Location '$ScriptDir\backend';
    `$env:GOFLAGS='';
    Write-Host '正在编译并启动后端...' -ForegroundColor Cyan;
    try { go run . } catch { Write-Host `$_ -ForegroundColor Red; Read-Host '按回车退出' }
"@ -PassThru -WindowStyle Normal
Success "后端进程启动 PID=$($backendProc.Id)"

Write-Host "  等待后端 API 就绪..." -ForegroundColor Gray
$apiOk = $false
for ($i = 0; $i -lt 30; $i++) {
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:8080/api/health" -UseBasicParsing -TimeoutSec 2
        if ($r.StatusCode -eq 200) { $apiOk = $true; break }
    } catch {}
    Start-Sleep -Seconds 1
}
if ($apiOk) { Success "后端 API 已就绪 http://localhost:8080" } else { Warn "后端仍在启动..." }

# ---------------------------------------------------------------------------
Step "Step 6/6: 启动传感器数据模拟器 (回填 365 天历史数据)"
$simProc = Start-Process powershell -ArgumentList @"
    -NoExit, -Command,
    `$Host.UI.RawUI.WindowTitle='传感器模拟器 Sensor Simulator';
    Set-Location '$ScriptDir\simulator';
    Write-Host '模拟器将回填 365 天历史数据，请耐心等待...' -ForegroundColor Cyan;
    Write-Host '回填完成后进入实时模拟模式 (每10秒模拟+1小时数据)' -ForegroundColor Yellow;
    try { go run . } catch { Write-Host `$_ -ForegroundColor Red; Read-Host '按回车退出' }
"@ -PassThru -WindowStyle Normal
Success "模拟器进程启动 PID=$($simProc.Id)"

# ---------------------------------------------------------------------------
Write-Host ""
Write-Host "═══════════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🏛️  系统已启动！" -ForegroundColor Green
Write-Host "═══════════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""
Write-Host "  📊 前端页面:" -NoNewline -ForegroundColor White
Write-Host "  frontend/index.html (直接用浏览器打开)"
Write-Host "  🌐 后端 API:" -NoNewline -ForegroundColor White
Write-Host "  http://localhost:8080"
Write-Host "  📡 MQTT Broker:" -NoNewline -ForegroundColor White
Write-Host " tcp://localhost:1883 (告警主题: aqueduct/alerts/#)"
Write-Host "  📋 MQTT 浏览:" -NoNewline -ForegroundColor White
Write-Host "  http://localhost:4000 (MQTT Explorer)"
Write-Host "  🗄️  数据库:" -NoNewline -ForegroundColor White
Write-Host "   postgresql://localhost:5432/aqueduct_monitor"
Write-Host ""
Write-Host "  ⌨️  快捷操作:"
Write-Host "     1. 打开前端：" -NoNewline
Write-Host "     explorer frontend\index.html"
Write-Host "     2. 触发评估： POST /api/evaluation/run"
Write-Host ""
Write-Host "  停止所有后端服务请关闭对应的 PowerShell 窗口"
Write-Host "═══════════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

Start-Process "frontend\index.html"
