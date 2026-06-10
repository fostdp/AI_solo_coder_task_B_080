-- ============================================
-- 古罗马水道工程结构健康监测系统
-- TimescaleDB 初始化脚本
-- ============================================

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================
-- 1. 水道基本信息表
-- ============================================
CREATE TABLE IF NOT EXISTS aqueducts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    latin_name VARCHAR(100),
    construction_year INTEGER,
    length_km NUMERIC(10,3),
    height_m NUMERIC(8,2),
    start_location VARCHAR(200),
    end_location VARCHAR(200),
    description TEXT,
    geo_path JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 11条古罗马著名水道
INSERT INTO aqueducts (name, latin_name, construction_year, length_km, height_m, start_location, end_location, description, geo_path) VALUES
('Appia水道', 'Aqua Appia', -312, 16.400, 15.0, '罗马东郊', '罗马Forum', '古罗马第一条水道', 
 '{"segments": [{"type": "arcade", "count": 20, "span": 5.5}, {"type": "tunnel", "length": 8000, "height": 2.5}]}'),
('Anio Vetus水道', 'Aqua Anio Vetus', -272, 63.700, 25.0, 'Tivoli附近', '罗马Esquiline山', '古老的石砌水道', 
 '{"segments": [{"type": "arcade", "count": 85, "span": 6.0}, {"type": "tunnel", "length": 35000, "height": 2.8}]}'),
('Marta水道', 'Aqua Marcia', -144, 91.300, 30.0, 'Subiaco附近', '罗马Capitoline山', '最高的古罗马水道之一', 
 '{"segments": [{"type": "arcade", "count": 120, "span": 6.5}, {"type": "tunnel", "length": 50000, "height": 3.0}]}'),
('Tepula水道', 'Aqua Tepula', -126, 18.000, 22.0, '罗马东南', '罗马Aventine山', '温水供应水道', 
 '{"segments": [{"type": "arcade", "count": 45, "span": 5.0}, {"type": "tunnel", "length": 8000, "height": 2.2}]}'),
('Julia水道', 'Aqua Julia', -33, 22.000, 25.0, 'Gabii湖', '罗马Viminal山', 'Agrippa修建的水道', 
 '{"segments": [{"type": "arcade", "count": 60, "span": 5.8}, {"type": "tunnel", "length": 10000, "height": 2.5}]}'),
('Virgo水道', 'Aqua Virgo', -19, 21.000, 18.0, '罗马东北', '罗马Campus Martius', '几乎完整保存的水道', 
 '{"segments": [{"type": "arcade", "count": 35, "span": 5.2}, {"type": "tunnel", "length": 12000, "height": 2.4}]}'),
('Alsietina水道', 'Aqua Alsietina', 2, 32.800, 12.0, 'Alsietinus湖', '罗马Trastevere', '主要供应花园和喷泉', 
 '{"segments": [{"type": "arcade", "count": 28, "span": 4.8}, {"type": "tunnel", "length": 18000, "height": 2.0}]}'),
('Claudia水道', 'Aqua Claudia', 52, 68.700, 33.0, 'Subiaco', '罗马Caelian山', '最大的拱券水道之一', 
 '{"segments": [{"type": "arcade", "count": 150, "span": 7.0}, {"type": "tunnel", "length": 40000, "height": 3.2}]}'),
('Anio Novus水道', 'Aqua Anio Novus', 38, 86.800, 28.0, 'Anio河上游', '罗马Caelian山', '水量最大的水道', 
 '{"segments": [{"type": "arcade", "count": 95, "span": 6.2}, {"type": "tunnel", "length": 55000, "height": 2.9}]}'),
('Traiana水道', 'Aqua Traiana', 109, 56.800, 20.0, 'Bolsena湖', '罗马Janiculum山', '图拉真皇帝修建', 
 '{"segments": [{"type": "arcade", "count": 72, "span": 5.6}, {"type": "tunnel", "length": 30000, "height": 2.6}]}'),
('Severiana水道', 'Aqua Severiana', 226, 32.900, 15.0, '罗马东南', '罗马Lateran', '最后一条帝国水道', 
 '{"segments": [{"type": "arcade", "count": 40, "span": 5.0}, {"type": "tunnel", "length": 15000, "height": 2.3}]}');

-- ============================================
-- 2. 水道结构段表（桥墩、拱券分段）
-- ============================================
CREATE TABLE IF NOT EXISTS structure_segments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_type VARCHAR(20) NOT NULL CHECK (segment_type IN ('pier', 'arch', 'tunnel', 'channel')),
    segment_index INTEGER NOT NULL,
    position_geo JSONB,
    position_3d JSONB,
    design_strength NUMERIC(12,4),
    original_material VARCHAR(100),
    design_load_capacity NUMERIC(12,4),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(aqueduct_id, segment_type, segment_index)
);

-- 生成示例结构段数据 - 每条水道生成桥墩和拱券
DO $$
DECLARE
    aq RECORD;
    pier_count INTEGER;
    arch_count INTEGER;
    i INTEGER;
    j INTEGER;
    base_x NUMERIC := 0;
BEGIN
    FOR aq IN SELECT id, geo_path FROM aqueducts LOOP
        IF aq.geo_path IS NOT NULL AND aq.geo_path->'segments' IS NOT NULL THEN
            i := 1;
            FOR j IN 0..((aq.geo_path->'segments'->0->>'count')::INTEGER - 1) LOOP
                INSERT INTO structure_segments (aqueduct_id, segment_type, segment_index, position_geo, position_3d, design_strength, original_material, design_load_capacity)
                VALUES (aq.id, 'pier', j + 1,
                    jsonb_build_object('lat', 41.9028 + (j::NUMERIC / 1000), 'lng', 12.4964 + (j::NUMERIC / 1000)),
                    jsonb_build_object('x', (j::NUMERIC) * 6.0, 'y', 0, 'z', 0, 'height', 15 + (random() * 15)),
                    25.0 + random() * 10.0,
                    '石灰华石材',
                    500.0 + random() * 300.0
                );

                IF j < ((aq.geo_path->'segments'->0->>'count')::INTEGER - 1) THEN
                    INSERT INTO structure_segments (aqueduct_id, segment_type, segment_index, position_geo, position_3d, design_strength, original_material, design_load_capacity)
                    VALUES (aq.id, 'arch', j + 1,
                        jsonb_build_object('lat', 41.9028 + ((j + 0.5)::NUMERIC / 1000), 'lng', 12.4964 + ((j + 0.5)::NUMERIC / 1000)),
                        jsonb_build_object('x', (j::NUMERIC) * 6.0 + 3.0, 'y', 12.0, 'z', 0, 'span', 5.5 + random()),
                        20.0 + random() * 8.0,
                        '古罗马混凝土',
                        350.0 + random() * 200.0
                    );
                END IF;
            END LOOP;
        END IF;
    END LOOP;
END $$;

-- ============================================
-- 3. 传感器注册表
-- ============================================
CREATE TABLE IF NOT EXISTS sensors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sensor_code VARCHAR(50) UNIQUE NOT NULL,
    segment_id UUID NOT NULL REFERENCES structure_segments(id) ON DELETE CASCADE,
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    sensor_type VARCHAR(30) NOT NULL CHECK (sensor_type IN ('stress', 'weathering', 'settlement', 'tilt', 'temperature', 'humidity')),
    location_description VARCHAR(200),
    installed_date DATE,
    sampling_interval_sec INTEGER DEFAULT 3600,
    calibration_date DATE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 为每个结构段生成传感器
DO $$
DECLARE
    seg RECORD;
BEGIN
    FOR seg IN SELECT id, aqueduct_id, segment_type FROM structure_segments LOOP
        IF seg.segment_type = 'arch' THEN
            INSERT INTO sensors (sensor_code, segment_id, aqueduct_id, sensor_type, location_description, installed_date)
            VALUES 
                ('STRS-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'stress', '拱券拱顶位置', CURRENT_DATE - INTERVAL '365 days'),
                ('WTHR-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'weathering', '拱券砂浆接缝处', CURRENT_DATE - INTERVAL '365 days'),
                ('TILT-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'tilt', '拱券拱脚位置', CURRENT_DATE - INTERVAL '365 days');
        ELSIF seg.segment_type = 'pier' THEN
            INSERT INTO sensors (sensor_code, segment_id, aqueduct_id, sensor_type, location_description, installed_date)
            VALUES 
                ('STRS-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'stress', '桥墩中下部', CURRENT_DATE - INTERVAL '365 days'),
                ('STLM-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'settlement', '桥墩基础', CURRENT_DATE - INTERVAL '365 days'),
                ('TMPR-' || substr(seg.id::text, 1, 8), seg.id, seg.aqueduct_id, 'temperature', '桥墩表面', CURRENT_DATE - INTERVAL '365 days');
        END IF;
    END LOOP;
END $$;

-- ============================================
-- 4. 传感器时序数据表 (使用TimescaleDB超表)
-- ============================================
CREATE TABLE IF NOT EXISTS sensor_data (
    sensor_id UUID NOT NULL REFERENCES sensors(id) ON DELETE CASCADE,
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES structure_segments(id) ON DELETE CASCADE,
    sensor_type VARCHAR(30) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    value NUMERIC(16,6) NOT NULL,
    unit VARCHAR(20) NOT NULL,
    quality SMALLINT DEFAULT 1,
    dtu_id VARCHAR(50),
    rssi NUMERIC(5,1),
    PRIMARY KEY (sensor_id, timestamp)
);

-- 创建超表（按时间分区）
SELECT create_hypertable('sensor_data', 'timestamp', 
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sensor_data_aqueduct_time ON sensor_data (aqueduct_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_data_segment_time ON sensor_data (segment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_data_type_time ON sensor_data (sensor_type, timestamp DESC);

-- ============================================
-- 5. 结构评估结果表
-- ============================================
CREATE TABLE IF NOT EXISTS structural_evaluations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES structure_segments(id) ON DELETE CASCADE,
    evaluation_time TIMESTAMPTZ NOT NULL,
    current_stress NUMERIC(12,4),
    max_stress NUMERIC(12,4),
    weathering_depth NUMERIC(10,4),
    settlement_mm NUMERIC(10,4),
    residual_strength NUMERIC(12,4),
    residual_capacity_ratio NUMERIC(6,4),
    safety_level VARCHAR(10) CHECK (safety_level IN ('SAFE', 'WARNING', 'DANGER', 'CRITICAL')),
    fea_model_data JSONB,
    recommendations TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evaluations_aqueduct_time ON structural_evaluations (aqueduct_id, evaluation_time DESC);
CREATE INDEX IF NOT EXISTS idx_evaluations_segment_time ON structural_evaluations (segment_id, evaluation_time DESC);
CREATE INDEX IF NOT EXISTS idx_evaluations_safety ON structural_evaluations (safety_level, evaluation_time DESC);

-- ============================================
-- 6. 告警记录表
-- ============================================
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES structure_segments(id) ON DELETE SET NULL,
    sensor_id UUID REFERENCES sensors(id) ON DELETE SET NULL,
    alert_type VARCHAR(40) NOT NULL CHECK (alert_type IN (
        'SETTLEMENT_EXCEEDED',
        'STRESS_EXCEEDED',
        'WEATHERING_ACCELERATED',
        'TILT_EXCEEDED',
        'LOAD_CAPACITY_LOW',
        'SENSOR_OFFLINE',
        'EQUIPMENT_FAULT'
    )),
    severity VARCHAR(10) NOT NULL CHECK (severity IN ('INFO', 'WARNING', 'CRITICAL', 'EMERGENCY')),
    title VARCHAR(200) NOT NULL,
    description TEXT,
    threshold_value NUMERIC(16,4),
    measured_value NUMERIC(16,4),
    unit VARCHAR(20),
    mqtt_published BOOLEAN DEFAULT false,
    mqtt_message_id VARCHAR(100),
    acknowledged BOOLEAN DEFAULT false,
    acknowledged_by VARCHAR(100),
    acknowledged_at TIMESTAMPTZ,
    resolution_notes TEXT,
    resolved BOOLEAN DEFAULT false,
    resolved_at TIMESTAMPTZ,
    triggered_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_aqueduct ON alerts (aqueduct_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts (severity, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_resolved ON alerts (resolved, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_mqtt ON alerts (mqtt_published, severity);

-- ============================================
-- 7. 修复材料数据库
-- ============================================
CREATE TABLE IF NOT EXISTS repair_materials (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(150) NOT NULL,
    material_type VARCHAR(40) NOT NULL CHECK (material_type IN ('ROMAN_CONCRETE', 'MODERN_CEMENT', 'EPOXY', 'GROUT', 'FRP', 'STONE_PATCH', 'LIME_MORTAR')),
    composition JSONB,
    compressive_strength NUMERIC(10,2),
    tensile_strength NUMERIC(10,2),
    elastic_modulus NUMERIC(12,2),
    durability_rating NUMERIC(4,2),
    compatibility_rating NUMERIC(4,2),
    cost_per_unit NUMERIC(12,2),
    unit VARCHAR(20),
    ease_of_application NUMERIC(4,2),
    environmental_impact NUMERIC(4,2),
    aesthetic_match NUMERIC(4,2),
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 古罗马传统材料
INSERT INTO repair_materials (name, material_type, composition, compressive_strength, tensile_strength, elastic_modulus, durability_rating, compatibility_rating, cost_per_unit, unit, ease_of_application, environmental_impact, aesthetic_match, description) VALUES
('古罗马混凝土配方A (标准)', 'ROMAN_CONCRETE', 
 '{"lime": 1, "pozzolana": 2, "tufa_aggregate": 4, "water": 1.2}',
 20.5, 2.1, 24.5, 9.2, 9.8, 85.0, 'm³', 6.5, 1.5, 9.5,
 '标准Opus Caementicium配方，石灰与火山灰比例1:2'),
('古罗马混凝土配方B (高强度)', 'ROMAN_CONCRETE',
 '{"lime": 1, "pozzolana": 1.5, "brick_rubble": 3, "water": 0.9}',
 28.8, 3.2, 32.0, 9.5, 9.5, 120.0, 'm³', 5.0, 1.8, 9.0,
 '高石灰比例，碎砖骨料，适合承重结构修复'),
('古罗马混凝土配方C (水下)', 'ROMAN_CONCRETE',
 '{"lime": 1.5, "pozzolana": 1, "pumice": 2, "water": 0.8}',
 24.0, 2.5, 28.0, 9.8, 9.6, 150.0, 'm³', 4.5, 1.2, 8.8,
 '高火山灰含量，适合潮湿环境和基础修复'),
('传统石灰砂浆', 'LIME_MORTAR',
 '{"lime_putty": 3, "sand": 9, "water": 1}',
 3.5, 0.4, 2.8, 8.5, 9.9, 45.0, 'm³', 9.0, 1.1, 9.8,
 '纯石灰砂浆，透气性极佳，勾缝和表面修复首选');

-- 现代修复材料
INSERT INTO repair_materials (name, material_type, composition, compressive_strength, tensile_strength, elastic_modulus, durability_rating, compatibility_rating, cost_per_unit, unit, ease_of_application, environmental_impact, aesthetic_match, description) VALUES
('CEM II/B-L 32.5N 低碱水泥', 'MODERN_CEMENT',
 '{"clinker": 0.65, "limestone": 0.30, "gypsum": 0.05}',
 52.0, 5.5, 35.0, 8.2, 5.5, 320.0, 'm³', 8.5, 4.5, 6.0,
 '低碱硅酸盐水泥，减少对古石材的碱骨料反应'),
('钛酸钾镁磷酸盐水泥', 'MODERN_CEMENT',
 '{"magnesia": 0.40, "potassium_phosphate": 0.35, "fly_ash": 0.20, "borax_retarder": 0.05}',
 65.0, 8.2, 42.0, 9.0, 7.8, 580.0, 'm³', 7.0, 3.2, 5.5,
 '快速固化水泥，适合紧急加固修复'),
('碳纤维增强聚合物 CFRP', 'FRP',
 '{"carbon_fiber": 0.68, "epoxy_resin": 0.30, "hardener": 0.02}',
 120.0, 85.0, 155.0, 9.5, 4.0, 8500.0, 'm²', 4.5, 6.8, 3.5,
 '高抗拉强度FRP片材，用于拱券和梁的结构加固'),
('玻璃纤维增强聚合物 GFRP', 'FRP',
 '{"glass_fiber": 0.60, "vinylester_resin": 0.38, "catalyst": 0.02}',
 75.0, 45.0, 85.0, 8.8, 4.2, 3200.0, 'm²', 6.0, 5.5, 4.0,
 '耐腐蚀GFRP，适合潮湿环境加固'),
('环氧树脂注浆料', 'EPOXY',
 '{"bisphenol_a_epoxy": 0.60, "polyamine_hardener": 0.35, "silica_filler": 0.05}',
 85.0, 12.0, 32.0, 9.0, 6.5, 1200.0, 'kg', 5.5, 7.2, 3.0,
 '高粘结强度环氧注浆，裂缝修补首选'),
('微膨胀水泥基注浆料', 'GROUT',
 '{"cement": 0.35, "expansive_agent": 0.08, "superplasticizer": 0.02, "sand": 0.55}',
 60.0, 4.5, 38.0, 8.5, 7.2, 280.0, 'm³', 7.5, 5.0, 5.8,
 '无收缩注浆，用于基础沉降加固和空洞填充'),
('石灰华修补砂浆', 'STONE_PATCH',
 '{"lime": 0.15, "travertine_dust": 0.60, "pozzolana": 0.10, "acrylic_polymer": 0.05, "water": 0.10}',
 18.0, 1.8, 22.0, 8.8, 9.2, 480.0, 'm³', 7.0, 2.8, 9.5,
 '以石灰华粉为骨料的修补砂浆，外观与原石材高度匹配');

-- ============================================
-- 8. 修复方案推荐记录
-- ============================================
CREATE TABLE IF NOT EXISTS repair_recommendations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES structure_segments(id) ON DELETE CASCADE,
    evaluation_id UUID REFERENCES structural_evaluations(id) ON DELETE SET NULL,
    recommendation_time TIMESTAMPTZ NOT NULL,
    damage_type VARCHAR(50),
    damage_severity NUMERIC(4,2),
    recommended_materials JSONB,
    decision_scores JSONB,
    expected_cost NUMERIC(14,2),
    expected_lifespan_years INTEGER,
    construction_notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_recommendations_aqueduct ON repair_recommendations (aqueduct_id, recommendation_time DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_segment ON repair_recommendations (segment_id, recommendation_time DESC);

-- ============================================
-- 9. 连续聚合视图 - 每日传感器数据统计
-- ============================================
CREATE MATERIALIZED VIEW sensor_data_daily
WITH (timescaledb.continuous) AS
SELECT
    sensor_id,
    time_bucket('1 day', timestamp) AS bucket,
    AVG(value) AS avg_value,
    MAX(value) AS max_value,
    MIN(value) AS min_value,
    STDDEV(value) AS stddev_value,
    COUNT(*) AS sample_count
FROM sensor_data
GROUP BY sensor_id, time_bucket('1 day', timestamp)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sensor_data_daily',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- ============================================
-- 10. 连续聚合视图 - 每小时统计
-- ============================================
CREATE MATERIALIZED VIEW sensor_data_hourly
WITH (timescaledb.continuous) AS
SELECT
    sensor_id,
    time_bucket('1 hour', timestamp) AS bucket,
    AVG(value) AS avg_value,
    MAX(value) AS max_value,
    MIN(value) AS min_value,
    FIRST(value, timestamp) AS first_value,
    LAST(value, timestamp) AS last_value
FROM sensor_data
GROUP BY sensor_id, time_bucket('1 hour', timestamp)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('sensor_data_hourly',
    start_offset => INTERVAL '1 day',
    end_offset => INTERVAL '10 minutes',
    schedule_interval => INTERVAL '30 minutes',
    if_not_exists => TRUE
);
