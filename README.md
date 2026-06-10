# 🏛️ 古罗马水道工程结构健康与现代修复评估系统

Aqueduct Structural Health Monitoring & Modern Rehabilitation Decision Support System

某考古工程团队对古罗马11条水道的现存遗迹进行了详细测绘，每条水道布设了结构应力传感器和位移计（模拟定期上传），每1小时通过4G DTU上报拱券应力、砂浆风化深度、基础沉降量等数据。

---

## ✨ 系统功能特性

### 1. 数据采集层
- **4G DTU数据上报接口**：支持批量传感器读数提交，带质量标识和信号强度RSSI
- **传感器类型**：应力计(MPa)、风化深度(mm)、沉降计(mm)、倾角计(°)、温湿度计
- **时序数据存储**：TimescaleDB超表 + 7天自动分区 + 日/小时连续聚合视图

### 2. 结构健康监测可视化（Three.js + Canvas）
- **11条水道三维重建**：参数化桥墩（基座+柱身+柱头）、半圆拱券（分块楔形石）、明渠水槽
- **动态着色映射**：
  - 🟢→🔴 风化深度配色（0mm ~ 40mm+）
  - 按承载力剩余比例（SAFE/WARNING/DANGER/CRITICAL）配色
- **Canvas辅助**：小地图顶视图、趋势折线图（近1年，支持应力/风化/沉降）
- **交互**：悬停提示、点击结构段弹出详细面板、轨道控制/线框模式切换

### 3. 结构安全评估模型
基于**有限元简化模型 (Simplified FEA)** + **材料退化曲线**：

#### 三维结构建模
```
桥墩：6节点空间框架（2段柱身+横向联系梁），1.2m × 2.5m截面
拱券：9节点抛物线拱轴线，8梁单元 + 分段楔形石体积模型
```

#### 承载力计算因子
- **风化退化**：截面损失率 → 强度折减（幂函数 1.3 次方）
- **沉降影响**：差异沉降 → 倾斜偏心 → 附加弯曲应力
- **老化因子**：2000年服役寿命指数衰减
- **应力集中**：拱脚/铰区 1.15x ~ 1.6x 放大

#### 五级预警机制
| 安全等级 | 承载力比值 | 触发响应 |
|---------|----------|---------|
| SAFE | ≥80% | 常规监测 |
| WARNING | ≥65% | 加强巡检 |
| DANGER | ≥50% | 列入年度加固计划 |
| CRITICAL | <50% | **紧急加固 + 封闭交通** |

### 4. 修复方案推荐引擎（TOPSIS多属性决策）
基于**古罗马传统配方**（石灰-火山灰-碎砖三元体系）和**现代材料库**（11种）：

#### 古罗马混凝土配方
| 配方 | 石灰：火山灰：骨料 | 抗压强度 | 适用场景 |
|------|-----------------|---------|---------|
| 标准A | 1:2:4 | 20.5 MPa | 普通结构修复 |
| 高强度B | 1:1.5:3 | 28.8 MPa | 承重结构 |
| 水下C | 1.5:1:2(浮石) | 24.0 MPa | 基础/潮湿区 |

#### MADM决策属性权重（文物保护场景自动调整）
| 属性 | 基准权重 | 承重关键 | 文物优先 |
|-----|---------|---------|---------|
| 抗压强度 | 12% | +8% | — |
| 抗拉强度 | 10% | +6% | — |
| 耐久性 | 15% | +3% | — |
| **原结构相容性** | 18% | — | **+7%** |
| **美学匹配** | 14% | — | **+6%** |
| **环境影响** | 7% | — | +4% |
| 成本/施工便捷 | 18% | — | — |

> 遵循《威尼斯宪章》最小干预原则，古罗马/石灰材料自动附加+12%/+8%遗产友好加成

### 5. 告警与MQTT推送
- **7类告警**：沉降超限、应力超限、风化加速、倾角超限、承载力不足、传感器离线、设备故障
- **MQTT分级主题**：`aqueduct/alerts/{严重级别}/{水道ID}/{告警类型}`
- **QoS=1** + 持久会话 + CRITICAL自动RETAIN
- 兼容MQTT Explorer可视化（Docker内置4000端口）

---

## 📁 项目结构

```
AI_solo_coder_task_A_080/
├── backend/                    # Go 后端服务
│   ├── main.go                 # 入口 + 路由 + 定时评估Cron
│   ├── go.mod                  # 依赖：Gin/pgx/v5/paho.mqtt/gonum
│   ├── .env                    # 环境配置（数据库/MQTT/阈值）
│   ├── config/                 # 配置加载
│   ├── database/               # TimescaleDB连接池
│   ├── models/                 # 数据模型（20+实体）
│   ├── repository/             # SQL数据访问层
│   ├── handlers/               # HTTP API处理器
│   ├── evaluation/             # ⭐ FEA结构评估核心
│   ├── recommendation/         # ⭐ TOPSIS修复推荐引擎
│   └── mqtt/                   # MQTT告警发布器
│
├── simulator/                  # 传感器数据模拟器
│   ├── main.go                 # 365天历史回填 + 实时模拟
│   └── go.mod
│
├── frontend/                   # 前端（Three.js + Canvas）
│   ├── index.html              # 入口
│   ├── css/style.css           # 古罗马金+深蓝配色主题
│   └── js/
│       ├── api.js              # API封装+格式化工具
│       ├── viz.js              # ⭐ Three.js 3D场景
│       └── app.js              # ⭐ 面板/图表/业务逻辑
│
├── database/
│   └── init.sql                # TimescaleDB DDL（10+表，2条连续聚合）
│
├── mqtt/
│   └── mosquitto.conf          # Eclipse Mosquitto配置
│
├── docker-compose.yml          # 一键启动：TimescaleDB + MQTT + MQTTExplorer
├── start.ps1                   # Windows一键启动脚本（Go后端+模拟器+前端）
└── README.md
```

---

## 🚀 快速开始

### 前置要求
- **Docker Desktop** (>=4.20) - 运行 TimescaleDB、MQTT Broker
- **Go** (>=1.21) - 编译后端和模拟器
- **现代浏览器** - Chrome/Edge/Firefox（支持 ES Modules 和 WebGL）

### 方式一：一键启动（推荐）
```powershell
# 在项目根目录
powershell -ExecutionPolicy Bypass -File start.ps1
```
脚本会自动：
1. ✅ Docker Compose 启动 TimescaleDB(5432)、Mosquitto(1883)、MQTT-Explorer(4000)
2. ✅ 自动执行 `init.sql` 初始化11条水道 + 传感器 + 材料库
3. ✅ 编译启动 Go 后端 (8080端口)
4. ✅ 启动模拟器：回填365天历史 + 进入实时模拟模式
5. ✅ 自动打开浏览器加载前端

### 方式二：分步手动启动
```powershell
# 1. 启动基础设施
docker compose up -d --wait

# 2. 启动后端（5432端口就绪后）
cd backend
go mod tidy
go run .

# 3. 新开窗口：启动模拟器（先回填再实时）
cd simulator
go run .

# 4. 打开前端
explorer frontend\index.html
```

---

## 🌐 API 接口清单

### DTU数据采集
| Method | 路径 | 描述 |
|--------|-----|------|
| POST | `/api/dtu/submit` | DTU批量提交传感器读数 |

### 数据查询
| Method | 路径 | 描述 |
|--------|-----|------|
| GET | `/api/aqueducts` | 11条水道列表 |
| GET | `/api/aqueducts/:id` | 水道详情+结构段+传感器+告警 |
| GET | `/api/segments[?aqueduct_id=]` | 结构段列表（含承载力着色数据） |
| GET | `/api/segments/:id` | 结构段详情 + 近1年3类趋势数据 |
| GET | `/api/segments/:id/repair` | ⭐ **修复方案推荐结果** |
| GET | `/api/sensors/:id/trend` | 传感器历史趋势（可调粒度） |
| GET | `/api/materials` | 修复材料数据库 |
| GET | `/api/alerts` | 活动告警列表（按严重度排序） |
| GET | `/api/stats` | 首页概览统计指标 |
| POST | `/api/evaluation/run` | 手动触发全段评估 |
| GET | `/api/health` | 健康检查 |

---

## 🧪 典型使用流程

1. **启动系统** → 等待模拟器回填完365天数据（约2-5分钟，后端窗口可见进度条）
2. **打开前端** → 左侧点击任一条水道（如 *Claudia水道* 最高的拱券水道）
3. **观察3D模型**：
   - 🎨 切换"风化深度/承载力"配色模式
   - 📐 切换线框模式检查内部结构
   - 🖱️ 悬停结构看实时tooltip
4. **点击高风险红色段** → 右侧面板查看：
   - 承载力比值进度条（<50%闪红）
   - 近1年应力/风化/沉降趋势图
   - 风化速率加速指示
5. **切换【修复推荐】Tab** → 查看：
   - TOPSIS多属性决策排名（★最佳推荐）
   - 古罗马配方 vs 现代材料属性对比
   - 造价估算、预期寿命、施工建议
6. **切换【告警】Tab** → 查看MQTT实时推送到文物保护中心的告警

---

## 🎯 算法核心说明

### 结构承载力计算核心公式
```
有效强度 f'_c = f_c × η_weathering × η_settlement × η_aging
   weathering = (1 - d_wear/t_struct)^1.3  (风化截面损失)
   settlement = 1 - (Δ/2Δ_max)×0.25      (沉降折减)
   aging      = 0.85 + 0.15e^(-T/800)     (时间老化)

承载力比 R = f'_c / f_c,design
   ≥80% → SAFE / <50% → CRITICAL(加固预警)

应力校核（Simplified FEA）:
   拱顶轴力 N = qL²/(8f), 弯矩 M = qL²/8×(1-1.4f/L)
   墩柱偏心 e = H/2 × sin(Δ_rot), 弯曲应力 σ_b = N·e / W
   最大应力 σ_max = (N/A + |M|/W) × K_stress
```

### TOPSIS决策流程
```
1. 决策矩阵 X(n×m) → 向量归一化 X'_ij = X_ij/√ΣX²
2. 加权归一化 V_ij = w_j × X'_ij (文物模式自动加权调整)
3. 正负理想解 V⁺/V⁻（效益型取max/成本型取min）
4. 欧氏距离 D⁺_i/D⁻_i = √Σ(V-V⁺/⁻)²
5. 贴近度 C_i = D⁻_i/(D⁺_i + D⁻_i)
6. 遗产加成: 罗马混凝土 ×1.12, 石灰砂浆 ×1.08
7. 降序排序 C_i → TOP1 推荐
```

---

## 📜 11条古罗马水道

| 水道名 | 拉丁名 | 始建 | 长度(km) | 最高拱(m) | 备注 |
|-------|--------|-----|---------|----------|------|
| Appia | Aqua Appia | BC312 | 16.4 | 15.0 | 第一条水道 |
| Anio Vetus | Aqua Anio Vetus | BC272 | 63.7 | 25.0 | 早期石砌 |
| Marta | Aqua Marcia | BC144 | 91.3 | 30.0 | 最高之一 |
| Tepula | Aqua Tepula | BC126 | 18.0 | 22.0 | 温水供应 |
| Julia | Aqua Julia | BC33 | 22.0 | 25.0 | Agrippa建 |
| Virgo | Aqua Virgo | BC19 | 21.0 | 18.0 | 保存最佳 |
| Alsietina | Aqua Alsietina | AD2 | 32.8 | 12.0 | 花园喷泉 |
| Claudia | Aqua Claudia | AD52 | 68.7 | 33.0 | 最大拱券 |
| Anio Novus | Aqua Anio Novus | AD38 | 86.8 | 28.0 | 水量最大 |
| Traiana | Aqua Traiana | AD109 | 56.8 | 20.0 | 图拉真 |
| Severiana | Aqua Severiana | AD226 | 32.9 | 15.0 | 末代帝国水道 |

---

## 🔧 关键配置参数 (.env)

```ini
# 承载力加固预警阈值 (0.50 = 50%设计值)
LOAD_CAPACITY_THRESHOLD=0.50

# 沉降限值 (mm)
SETTLEMENT_LIMIT_MM=20.0

# 风化加速判定 (近期/长期 比值)
WEATHERING_ACCELERATION_RATIO=1.5
```

---

## 🛠️ 技术栈

| 层级 | 技术选型 |
|-----|---------|
| **后端语言** | Go 1.21 (Gin Web 框架) |
| **时序数据库** | PostgreSQL 15 + TimescaleDB 2.13 |
| **消息中间件** | Eclipse Mosquitto (MQTT 3.1.1) |
| **科学计算** | Gonum (数值矩阵运算, 预留接口) |
| **3D渲染** | Three.js r160 + OrbitControls |
| **2D图表** | 原生 Canvas API (折线/面积/进度图) |
| **数据模拟器** | Go 内置随机数生成 + 季节/日变化周期函数 |
| **部署** | Docker Compose + PowerShell 编排 |

---

## ⚠️ 故障排查

| 症状 | 排查命令 |
|-----|---------|
| 数据库连接失败 | `docker exec aqueduct-timescaledb pg_isready` |
| 初始化SQL未执行 | `docker logs aqueduct-timescaledb` |
| MQTT不通 | `docker exec aqueduct-mqtt mosquitto_sub -t 'aqueduct/#' -v` |
| 前端白屏 | 检查浏览器控制台，确认 importmap three.js 加载成功 |
| 3D场景黑屏 | 确认浏览器支持 WebGL: https://get.webgl.org |
| 模拟器历史数据慢 | 属正常现象，建议耐心等待前30%进度后会加速 |

---

> 🏺 **关于古罗马混凝土**：公元1世纪已广泛使用 *Opus Caementicium*，其自愈合特性（火山灰+海水→铝托贝莫来石晶体生成）使部分水道历经2000年仍屹立不倒，本系统的配方A/B/C忠实还原了Vitruvius《建筑十书》中的经典配合比。
