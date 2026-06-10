# 古罗马水道工程结构健康与现代修复评估系统

## 1. 系统架构

**系统全景图：**
```
                        ┌───────────────────┐
                        │   传感器模拟器     │
                        │  (11条水道/小时级) │
                        └─────────┬─────────┘
                                  │ DTU 4G HTTP
                                  ▼
                        ┌───────────────────┐
                        │   Go 后端服务      │◄── Prometheus 指标
                        │  ┌──────────────┐ │
                        │  │ dtu_receiver │ │  (pprof /metrics)
                        │  └──────┬───────┘ │
                        │         │  chan   │
                        │  ┌──────▼───────┐ │
                        │  │ structural_  │ │
                        │  │  evaluator   │ │  FEA 三铰拱+自适应网格
                        │  └──────┬───────┘ │
                        │         │  chan   │
                        │  ┌──────▼───────┐ │
                        │  │ repair_      │ │
                        │  │  advisor     │ │  TOPSIS+KNN+灵敏度
                        │  └──────┬───────┘ │
                        │         │  chan   │
                        │  ┌──────▼───────┐ │
                        │  │ alarm_       │ │
                        │  │  publisher   │ │  MQTT+离线队列+退避
                        │  └──────┬───────┘ │
                        └─────────┼─────────┘
                                  │ MQTT
                                  ▼
                        ┌───────────────────┐
                        │ Mosquitto MQTT    │───► 文物保护中心
                        └───────────────────┘
                                  ▲
                                  │ 查询
                        ┌───────────────────┐
                        │  前端三维可视化    │
                        │  (Three.js+LOD)   │
                        └───────────────────┘
```

**模块说明：**
- **dtu_receiver**: DTU传感器数据接收、验证、批量入库
- **structural_evaluator**: 三铰拱有限元分析 + 自适应网格 + 承载力计算
- **repair_advisor**: TOPSIS多属性决策 + KNN数据补全 + 灵敏度分析
- **alarm_publisher**: MQTT告警推送 + 离线队列 + 指数退避

**数据存储：**
- TimescaleDB: 传感器时序数据 + 压缩策略（30天自动压缩）
- 连续聚合: 小时级、日级聚合视图

## 2. 快速开始

### 2.1 环境要求
- Docker 24.0+
- Docker Compose 2.20+
- 至少 4GB RAM，推荐 8GB RAM

### 2.2 一键启动（核心服务）
```bash
cd AI_solo_coder_task_A_080
docker compose up -d
```

等待服务启动，访问：
- 前端界面: http://localhost:8080
- API 健康检查: http://localhost:8080/api/health
- pprof 调试: http://localhost:8080/debug/pprof
- Prometheus 指标: http://localhost:8080/metrics

### 2.3 启动传感器模拟器
```bash
docker compose --profile simulator up -d simulator
```

### 2.4 启动监控工具（Prometheus + MQTT Explorer）
```bash
docker compose --profile monitoring up -d
```

访问：
- Prometheus: http://localhost:9090
- MQTT Explorer: http://localhost:4000

### 2.5 停止服务
```bash
# 停止所有服务
docker compose down

# 停止含可选服务
docker compose --profile simulator --profile monitoring down

# 清理数据（谨慎使用）
docker compose down -v
```

## 3. 传感器模拟器用法

### 3.1 命令行参数

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `-aqueducts` | string | `all` | 要模拟的水道ID列表，逗号分隔 |
| `-interval` | int | `3600` | 上报间隔秒数（默认1小时） |
| `-backfill-days` | int | `365` | 历史回填天数 |
| `-inject-weathering` | float | `1.0` | 全局风化速率倍率注入 |
| `-inject-deformation` | float | `1.0` | 全局变形倍率注入 |
| `-seed` | int | `42` | 随机种子 |
| `-api-base` | string | `http://backend:8080/api` | 后端API地址 |
| `-realtime` | bool | `true` | 是否启动实时模拟 |
| `-list-aqueducts` | bool | `false` | 列出所有可用水道后退出 |

### 3.2 常用场景

**列出所有水道：**
```bash
docker run --rm aqueduct-simulator -list-aqueducts
```

**只模拟特定水道，回填30天：**
```bash
docker run --rm aqueduct-simulator -aqueducts "aq-001,aq-002" -backfill-days 30 -realtime=false
```

**注入高风化高变形的极端场景：**
```bash
docker run --rm aqueduct-simulator -inject-weathering 3.0 -inject-deformation 4.0 -seed 123
```

**加速模拟（15分钟间隔）：**
```bash
docker run --rm aqueduct-simulator -interval 900
```

### 3.3 11条水道独立配置

每条水道有独立的：
- 基准应力、位移、风化、沉降值
- 独立的异常事件概率
- 独立的传感器配置（6-12个监测点/水道）

可通过修改 simulator/main.go 中的 `initAqueductConfigs()` 进行定制。

## 4. 监控与性能调优

### 4.1 Prometheus 关键指标

| 指标名 | 说明 |
|---|---|
| `aqueduct_dtu_received_total` | 接收的DTU读数总数 |
| `aqueduct_eval_processed_total` | 完成的结构评估数 |
| `aqueduct_eval_duration_seconds` | 评估耗时直方图 |
| `aqueduct_fea_nonconverge_total` | FEA不收敛次数 |
| `aqueduct_alerts_published_total` | 发布的告警总数 |
| `aqueduct_alerts_buffered` | 离线队列当前大小 |
| `aqueduct_pipeline_queue_size` | 各阶段channel队列大小 |
| `aqueduct_sensor_value` | 最新传感器值 |
| `aqueduct_http_request_duration_seconds` | HTTP请求耗时 |

**Grafana 监控大盘推荐查询：**
```
# 评估吞吐量
rate(aqueduct_eval_processed_total[5m])

# FEA收敛失败率
rate(aqueduct_fea_nonconverge_total[1h]) / rate(aqueduct_eval_processed_total[1h])

# 管道队列堆积
aqueduct_pipeline_queue_size{stage="eval"}
```

### 4.2 pprof 性能分析

**采集 CPU profile 30秒：**
```bash
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
```

**采集内存 heap：**
```bash
go tool pprof http://localhost:8080/debug/pprof/heap
```

**采集 goroutine：**
```bash
go tool pprof http://localhost:8080/debug/pprof/goroutine
```

**查看火焰图：**
```bash
go tool pprof -http=:8081 profile.pprof
```

### 4.3 TimescaleDB 压缩策略

```sql
-- 查看压缩设置
SELECT hypertable_name, compression_state FROM timescaledb_information.hypertables;

-- 查看压缩块比例
SELECT 
  h.hypertable_name,
  (SELECT count(*) FROM show_chunks(h.hypertable_name)) AS total_chunks,
  (SELECT count(*) FROM show_chunks(h.hypertable_name, older_than => INTERVAL '30 days')) AS compressible_chunks,
  (SELECT count(*) FROM timescaledb_information.chunks c 
   WHERE c.hypertable_name = h.hypertable_name AND c.is_compressed) AS compressed_chunks
FROM timescaledb_information.hypertables h;

-- 手动压缩测试
SELECT compress_chunk(i, if_not_exists => TRUE)
FROM show_chunks('sensor_data', older_than => INTERVAL '7 days') i;

-- 修改压缩阈值
SELECT alter_job((SELECT job_id FROM timescaledb_information.jobs 
   WHERE proc_name = 'policy_compression' AND hypertable_name = 'sensor_data'),
   schedule_interval => INTERVAL '24 hours',
   config => '{"compress_after": "60 days"}');
```

### 4.4 前端 Gzip 压缩

后端已启用 gin-contrib/gzip 中间件，对所有静态资源和API响应自动压缩：
- 静态资源 (JS/CSS/HTML): 压缩率 ~70-85%
- API JSON 响应: 压缩率 ~60-75%

可通过浏览器开发者工具 Network 面板查看 `Content-Encoding: gzip` 验证。

## 5. 模型参数配置

所有模型参数已外置到 `backend/.env`，可通过环境变量覆盖：

### 5.1 FEA 有限元参数
```
FEA_MAX_ITERATIONS=50              # Newton-Raphson最大迭代次数
FEA_TOLERANCE=0.0001                # 收敛容差
FEA_RELAXATION=0.5                  # 松弛因子
FEA_MIN_ELEMENTS=6                  # 自适应网格最小单元数
FEA_MAX_ELEMENTS=32                 # 自适应网格最大单元数
FEA_CURVATURE_THRESH=0.15           # 曲率细分阈值
FEA_BASE_CONCRETE_FY=15.0           # 混凝土基准强度 MPa
FEA_WEATHERING_POWER=1.3            # 风化截面损失幂次
FEA_AQUEDUCT_AGE=2000               # 水道服役年限
```

### 5.2 MADM 多属性决策参数
```
MADM_KNN_NEIGHBORS=3                # K近邻补全K值
MADM_MISSING_PENALTY=0.7            # 缺失权重惩罚系数
MADM_MISSING_THRESHOLD=0.4          # 均值/KNN切换阈值
MADM_SENSITIVITY_PERTURB=0.20       # 灵敏度扰动幅度
```

### 5.3 Pipeline 并发配置
```
PIPE_BUFFER_SIZE=200                # Channel缓冲大小
PIPE_WORKERS_EVAL=4                 # FEA评估协程数
PIPE_WORKERS_REPAIR=2               # 推荐计算协程数
PIPE_WORKERS_ALARM=2                # 告警推送协程数
```

## 6. 故障排查

### 6.1 服务启动失败
```bash
# 查看所有服务状态
docker compose ps

# 查看后端日志
docker compose logs -f backend

# 查看数据库日志
docker compose logs -f timescaledb
```

### 6.2 MQTT 消息丢失
检查离线队列目录 `./mqtt_queue/`，查看持久化的 JSON 文件。

### 6.3 FEA 评估慢
- 检查 `aqueduct_eval_duration_seconds` 指标
- 增加 `PIPE_WORKERS_EVAL` 协程数
- 检查 `aqueduct_fea_nonconverge_total` 指标

### 6.4 数据库连接失败
确认 `.env` 中的 `TIMESCALE_HOST=timescaledb`，使用 Docker 服务名而非 localhost。

## 7. 目录结构

```
AI_solo_coder_task_A_080/
├── backend/                      # Go 后端服务
│   ├── main.go                   # 入口（集成pprof/gzip/metrics）
│   ├── Dockerfile                # 后端Dockerfile
│   ├── Dockerfile.simulator      # 模拟器Dockerfile
│   ├── .dockerignore
│   ├── .env                      # 环境配置
│   ├── go.mod/go.sum
│   ├── metrics/                  # Prometheus指标定义
│   ├── dtu_receiver/             # DTU数据接收模块
│   ├── structural_evaluator/     # 结构评估模块
│   ├── repair_advisor/           # 修复推荐模块
│   ├── alarm_publisher/          # 告警推送模块
│   ├── pipeline/                 # 管道组装器
│   ├── config/                   # 配置（参数外置）
│   ├── evaluation/               # FEA核心算法
│   ├── recommendation/           # 推荐算法
│   ├── mqtt/                     # MQTT客户端
│   ├── handlers/                 # API处理器
│   └── database/                 # 数据库连接
├── simulator/                    # 传感器模拟器
│   └── main.go
├── database/
│   └── init.sql                  # DB初始化 + 压缩策略
├── frontend/                     # 前端三维可视化
│   ├── index.html
│   ├── css/style.css
│   └── js/
│       ├── viz.js                # 兼容层
│       ├── aqueduct_3d_viewer.js # 3D视图类
│       └── repair_panel.js       # 修复面板类
├── mqtt/
│   └── mosquitto.conf
├── monitoring/                   # 监控配置
│   └── prometheus.yml
├── mqtt_queue/                   # MQTT离线队列持久化
├── docker-compose.yml            # 服务编排
├── start.ps1                     # Windows启动脚本
└── README.md
```

## 8. API 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/health` | 健康检查 |
| `GET` | `/metrics` | Prometheus 指标 |
| `GET` | `/debug/pprof/*` | pprof 调试 |
| `POST` | `/api/dtu/submit` | DTU传感器数据上报 |
| `GET` | `/api/aqueducts` | 水道列表 |
| `GET` | `/api/aqueducts/:id` | 水道详情 |
| `GET` | `/api/segments` | 结构段列表（3D模型用） |
| `GET` | `/api/segments/:id` | 结构段详情+1年趋势 |
| `GET` | `/api/segments/:id/repair` | 修复方案推荐 |
| `GET` | `/api/sensors/:id/trend` | 传感器历史趋势 |
| `GET` | `/api/alerts` | 告警列表 |
| `GET` | `/api/stats` | 综合统计 |
| `GET` | `/api/materials` | 修复材料库 |
| `POST` | `/api/evaluation/run` | 触发全量评估 |

---

**版本**: 1.0.0  
**上次更新**: 2026-06-10
