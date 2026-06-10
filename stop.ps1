# =============================================================================
# 古罗马水道结构健康监测系统 - 停止脚本
# Windows PowerShell
# =============================================================================

Write-Host "停止系统服务..." -ForegroundColor Red

Write-Host "`n[1] 停止后端和模拟器 PowerShell 窗口"
Get-Process powershell -ErrorAction SilentlyContinue | Where-Object {
    $_.MainWindowTitle -match 'Backend|后端|Sensor|模拟器' 
} | ForEach-Object {
    Write-Host "  终止 PID=$($_.Id) $($_.MainWindowTitle)" -ForegroundColor Yellow
    Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
}

Write-Host "`n[2] 停止 Go 编译运行进程"
Get-Process go -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process sensor-simulator -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Get-Process aqueduct-monitor -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

Write-Host "`n[3] 停止 Docker 容器 (保留数据卷)" -ForegroundColor Yellow
docker compose down

Write-Host "`n✅ 所有服务已停止。数据仍保存在 Docker 数据卷中。" -ForegroundColor Green
Write-Host "   如需彻底删除数据，执行: docker compose down -v"
