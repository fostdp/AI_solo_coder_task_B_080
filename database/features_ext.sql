-- ============================================
-- Feature扩展：古罗马混凝土耐久性反演、地震易损性、材料老化、旅游规划
-- 增量SQL，仅包含新表和新数据，不修改现有表
-- ============================================

-- ============================================
-- Feature 1: 古罗马混凝土耐久性反演
-- ============================================

-- 反演候选配方库
CREATE TABLE IF NOT EXISTS roman_concrete_formulas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    formula_name VARCHAR(100) NOT NULL,
    lime_ratio NUMERIC(5,4) NOT NULL,
    pozzolana_ratio NUMERIC(5,4) NOT NULL,
    aggregate_ratio NUMERIC(5,4) NOT NULL,
    water_ratio NUMERIC(5,4) NOT NULL,
    aggregate_type VARCHAR(50),
    additive_type VARCHAR(50),
    original_fy_mpa NUMERIC(8,4),
    original_fm_mpa NUMERIC(8,4),
    original_em_gpa NUMERIC(8,4),
    porosity NUMERIC(5,4),
    pore_size_distribution JSONB,
    durability_index NUMERIC(5,3),
    era_description VARCHAR(200),
    archaeological_sources TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 反演分析结果
CREATE TABLE IF NOT EXISTS concrete_inversion_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES structure_segments(id) ON DELETE CASCADE,
    analysis_time TIMESTAMPTZ NOT NULL,
    observed_weathering_depth NUMERIC(10,4),
    observed_strength NUMERIC(8,4),
    observed_mortar_ph NUMERIC(5,2),
    age_years NUMERIC(8,1),
    best_match_formula_id UUID REFERENCES roman_concrete_formulas(id),
    candidate_formulas JSONB,
    inversion_confidence NUMERIC(5,3),
    inferred_original_fy NUMERIC(8,4),
    inferred_durability_mechanism JSONB,
    leaching_rate NUMERIC(10,6),
    carbonation_depth NUMERIC(8,4),
    modern_reference_formula JSONB,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inversion_aqueduct ON concrete_inversion_results (aqueduct_id, analysis_time DESC);
CREATE INDEX IF NOT EXISTS idx_inversion_segment ON concrete_inversion_results (segment_id, analysis_time DESC);

-- 预置古罗马混凝土候选配方
INSERT INTO roman_concrete_formulas (formula_name, lime_ratio, pozzolana_ratio, aggregate_ratio, water_ratio, aggregate_type, original_fy_mpa, original_fm_mpa, original_em_gpa, porosity, durability_index, era_description, archaeological_sources) VALUES
('奥古斯都时期标准配方', 0.11, 0.22, 0.56, 0.11, '碎砖骨料', 20.5, 2.1, 24.5, 0.22, 0.92, '公元前1世纪-公元1世纪', 'Vitruvius De Architectura, Book 2'),
('克劳狄亚水道专用配方', 0.10, 0.25, 0.55, 0.10, '火山凝灰岩', 24.0, 2.5, 28.0, 0.20, 0.96, '公元1世纪中叶', 'Pozzuoli遗址考古发掘样品'),
('高强度承重配方', 0.13, 0.20, 0.58, 0.09, '石灰华碎块', 28.8, 3.2, 32.0, 0.18, 0.95, '帝国全盛期', 'Colosseum基础样品分析'),
('水下高火山灰配方', 0.12, 0.30, 0.50, 0.08, '浮石骨料', 26.5, 2.8, 30.0, 0.24, 0.98, '港口工程专用', 'Portus Cosanus沉船遗址'),
('石灰砂浆勾缝配方', 0.25, 0.08, 0.58, 0.09, '细砂', 3.5, 0.4, 2.8, 0.32, 0.85, '通用勾缝', '庞贝遗址墙体分析'),
('图拉真时期优化配方', 0.09, 0.27, 0.55, 0.09, '碎砖+火山灰', 25.8, 2.7, 29.5, 0.19, 0.97, '公元2世纪初', 'Trajans Market遗址');

-- ============================================
-- Feature 2: 地震易损性评估
-- ============================================

-- 区域历史地震记录
CREATE TABLE IF NOT EXISTS historical_earthquakes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_name VARCHAR(150),
    event_date DATE,
    magnitude NUMERIC(4,2),
    epicenter_lat NUMERIC(10,6),
    epicenter_lng NUMERIC(10,6),
    depth_km NUMERIC(8,2),
    intensity_msk NUMERIC(4,1),
    region VARCHAR(100),
    affected_aqueducts JSONB,
    historical_sources TEXT,
    damage_description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 地震易损性曲线
CREATE TABLE IF NOT EXISTS seismic_vulnerability (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES structure_segments(id) ON DELETE CASCADE,
    analysis_time TIMESTAMPTZ NOT NULL,
    damage_state VARCHAR(20) NOT NULL CHECK (damage_state IN ('NONE', 'SLIGHT', 'MODERATE', 'EXTENSIVE', 'COMPLETE')),
    magnitude NUMERIC(4,2),
    pga_g NUMERIC(8,5),
    probability NUMERIC(5,4),
    fragility_curve_params JSONB,
    capacity_spectrum JSONB,
    demand_spectrum JSONB,
    expected_repair_cost NUMERIC(14,2),
    expected_downtime_days INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_vulnerability_aqueduct ON seismic_vulnerability (aqueduct_id, analysis_time DESC);
CREATE INDEX IF NOT EXISTS idx_vulnerability_segment ON seismic_vulnerability (segment_id, analysis_time DESC);

-- 水道区域地震风险评级
CREATE TABLE IF NOT EXISTS aqueduct_seismic_risk (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID UNIQUE NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    region VARCHAR(100),
    peak_ground_accel_475yr NUMERIC(8,5),
    peak_ground_accel_2475yr NUMERIC(8,5),
    overall_risk_level VARCHAR(15) CHECK (overall_risk_level IN ('LOW', 'MODERATE', 'HIGH', 'VERY_HIGH')),
    site_class VARCHAR(5),
    soil_amplification NUMERIC(5,3),
    predominant_period_sec NUMERIC(6,3),
    vulnerable_segments INTEGER,
    estimated_total_loss NUMERIC(14,2),
    analysis_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 预置意大利中部历史地震
INSERT INTO historical_earthquakes (event_name, event_date, magnitude, epicenter_lat, epicenter_lng, depth_km, intensity_msk, region, damage_description) VALUES
('罗马51年地震', '0051-02-01', 5.8, 41.90, 12.50, 15.0, 7.5, '罗马城', 'Claudia水道部分拱券受损'),
('公元80年地震', '0080-09-15', 6.2, 42.10, 12.70, 20.0, 8.0, '拉齐奥北部', 'Anio Novus水道多处沉降'),
('1349年地震', '1349-09-09', 6.6, 41.75, 13.20, 18.0, 9.0, '拉丁姆', '多处水道拱券坍塌'),
('1695年地震', '1695-01-14', 6.0, 42.00, 12.55, 12.0, 7.0, '罗马近郊', 'Virgo水道局部受损'),
('1915年阿韦扎诺地震', '1915-01-13', 7.0, 42.05, 13.40, 15.0, 10.0, '阿布鲁佐', 'Marsica水道完全毁坏'),
('2009年拉奎拉地震', '2009-04-06', 6.3, 42.35, 13.38, 9.5, 8.5, '阿布鲁佐', '多处历史建筑受损'),
('2016年阿马特里切地震', '2016-08-24', 6.0, 42.70, 13.20, 8.0, 8.0, '翁布里亚', '区域古建筑受损');

-- ============================================
-- Feature 3: 修复材料长期性能预测
-- ============================================

-- 加速老化实验数据
CREATE TABLE IF NOT EXISTS accelerated_aging_data (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    material_id UUID NOT NULL REFERENCES repair_materials(id) ON DELETE CASCADE,
    test_type VARCHAR(30) NOT NULL CHECK (test_type IN ('FREEZE_THAW', 'WET_DRY', 'SULFATE', 'CARBONATION', 'CHLORIDE', 'HEAT_HUMIDITY')),
    temperature_c NUMERIC(6,2),
    humidity_pct NUMERIC(5,2),
    cycles INTEGER,
    exposure_days INTEGER,
    strength_retention NUMERIC(5,3),
    mass_loss_pct NUMERIC(6,3),
    elastic_modulus_loss NUMERIC(5,3),
    cracking_index NUMERIC(5,3),
    ph_change NUMERIC(5,2),
    test_notes TEXT,
    test_standard VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_aging_material ON accelerated_aging_data (material_id, test_type);

-- 材料长期性能预测结果
CREATE TABLE IF NOT EXISTS material_lifetime_prediction (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    material_id UUID NOT NULL REFERENCES repair_materials(id) ON DELETE CASCADE,
    prediction_time TIMESTAMPTZ NOT NULL,
    scenario VARCHAR(30) NOT NULL CHECK (scenario IN ('TEMPERATE', 'MEDITERRANEAN', 'COASTAL', 'ALPINE', 'URBAN_POLLUTED')),
    prediction_years INTEGER NOT NULL,
    arrhenius_activation_ev NUMERIC(6,3),
    time_temp_shift_factor JSONB,
    degradation_curve JSONB,
    strength_at_50yr NUMERIC(5,3),
    strength_at_100yr NUMERIC(5,3),
    estimated_service_life NUMERIC(6,1),
    threshold_strength_ratio NUMERIC(5,3),
    confidence_interval_low NUMERIC(5,3),
    confidence_interval_high NUMERIC(5,3),
    model_assumptions TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prediction_material ON material_lifetime_prediction (material_id, prediction_time DESC);

-- 预置加速老化基准数据
INSERT INTO accelerated_aging_data (material_id, test_type, temperature_c, humidity_pct, cycles, exposure_days, strength_retention, mass_loss_pct, elastic_modulus_loss, test_standard)
SELECT rm.id, 'FREEZE_THAW', -15.0, 60.0, 300, 100, 
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.88
        WHEN 'LIME_MORTAR' THEN 0.72
        WHEN 'MODERN_CEMENT' THEN 0.78
        WHEN 'EPOXY' THEN 0.65
        WHEN 'GROUT' THEN 0.75
        WHEN 'FRP' THEN 0.92
        WHEN 'STONE_PATCH' THEN 0.80
        ELSE 0.75
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 1.2
        WHEN 'LIME_MORTAR' THEN 3.5
        WHEN 'MODERN_CEMENT' THEN 2.1
        WHEN 'EPOXY' THEN 0.8
        WHEN 'GROUT' THEN 1.8
        WHEN 'FRP' THEN 0.3
        WHEN 'STONE_PATCH' THEN 2.0
        ELSE 1.5
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.10
        WHEN 'LIME_MORTAR' THEN 0.22
        WHEN 'MODERN_CEMENT' THEN 0.15
        WHEN 'EPOXY' THEN 0.28
        WHEN 'GROUT' THEN 0.18
        WHEN 'FRP' THEN 0.05
        WHEN 'STONE_PATCH' THEN 0.14
        ELSE 0.15
    END,
    'ASTM C666/C666M'
FROM repair_materials rm;

INSERT INTO accelerated_aging_data (material_id, test_type, temperature_c, humidity_pct, cycles, exposure_days, strength_retention, mass_loss_pct, elastic_modulus_loss, test_standard)
SELECT rm.id, 'WET_DRY', 40.0, 95.0, 500, 120,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.92
        WHEN 'LIME_MORTAR' THEN 0.68
        WHEN 'MODERN_CEMENT' THEN 0.70
        WHEN 'EPOXY' THEN 0.72
        WHEN 'GROUT' THEN 0.65
        WHEN 'FRP' THEN 0.88
        WHEN 'STONE_PATCH' THEN 0.75
        ELSE 0.72
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.8
        WHEN 'LIME_MORTAR' THEN 2.8
        WHEN 'MODERN_CEMENT' THEN 1.5
        WHEN 'EPOXY' THEN 1.2
        WHEN 'GROUT' THEN 2.2
        WHEN 'FRP' THEN 0.2
        WHEN 'STONE_PATCH' THEN 1.6
        ELSE 1.2
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.07
        WHEN 'LIME_MORTAR' THEN 0.18
        WHEN 'MODERN_CEMENT' THEN 0.12
        WHEN 'EPOXY' THEN 0.16
        WHEN 'GROUT' THEN 0.20
        WHEN 'FRP' THEN 0.04
        WHEN 'STONE_PATCH' THEN 0.10
        ELSE 0.12
    END,
    'ASTM D4799/D4799M'
FROM repair_materials rm;

INSERT INTO accelerated_aging_data (material_id, test_type, temperature_c, humidity_pct, cycles, exposure_days, strength_retention, mass_loss_pct, elastic_modulus_loss, test_standard)
SELECT rm.id, 'SULFATE', 23.0, 100.0, 0, 180,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.85
        WHEN 'LIME_MORTAR' THEN 0.55
        WHEN 'MODERN_CEMENT' THEN 0.60
        WHEN 'EPOXY' THEN 0.88
        WHEN 'GROUT' THEN 0.70
        WHEN 'FRP' THEN 0.95
        WHEN 'STONE_PATCH' THEN 0.65
        ELSE 0.70
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 1.5
        WHEN 'LIME_MORTAR' THEN 4.2
        WHEN 'MODERN_CEMENT' THEN 3.0
        WHEN 'EPOXY' THEN 0.5
        WHEN 'GROUT' THEN 2.5
        WHEN 'FRP' THEN 0.1
        WHEN 'STONE_PATCH' THEN 2.8
        ELSE 2.0
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.12
        WHEN 'LIME_MORTAR' THEN 0.30
        WHEN 'MODERN_CEMENT' THEN 0.25
        WHEN 'EPOXY' THEN 0.08
        WHEN 'GROUT' THEN 0.22
        WHEN 'FRP' THEN 0.03
        WHEN 'STONE_PATCH' THEN 0.18
        ELSE 0.17
    END,
    'ASTM C1012/C1012M'
FROM repair_materials rm;

INSERT INTO accelerated_aging_data (material_id, test_type, temperature_c, humidity_pct, cycles, exposure_days, strength_retention, mass_loss_pct, elastic_modulus_loss, test_standard)
SELECT rm.id, 'CARBONATION', 23.0, 65.0, 0, 365,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.94
        WHEN 'LIME_MORTAR' THEN 0.85
        WHEN 'MODERN_CEMENT' THEN 0.82
        WHEN 'EPOXY' THEN 0.98
        WHEN 'GROUT' THEN 0.80
        WHEN 'FRP' THEN 0.99
        WHEN 'STONE_PATCH' THEN 0.88
        ELSE 0.85
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.3
        WHEN 'LIME_MORTAR' THEN 0.5
        WHEN 'MODERN_CEMENT' THEN 0.8
        WHEN 'EPOXY' THEN 0.1
        WHEN 'GROUT' THEN 0.7
        WHEN 'FRP' THEN 0.0
        WHEN 'STONE_PATCH' THEN 0.4
        ELSE 0.5
    END,
    CASE rm.material_type
        WHEN 'ROMAN_CONCRETE' THEN 0.04
        WHEN 'LIME_MORTAR' THEN 0.08
        WHEN 'MODERN_CEMENT' THEN 0.10
        WHEN 'EPOXY' THEN 0.02
        WHEN 'GROUT' THEN 0.12
        WHEN 'FRP' THEN 0.01
        WHEN 'STONE_PATCH' THEN 0.06
        ELSE 0.07
    END,
    'BS EN 12390-12'
FROM repair_materials rm;

-- ============================================
-- Feature 4: 多水道对比与旅游规划
-- ============================================

-- 水道旅游评估参数
CREATE TABLE IF NOT EXISTS aqueduct_tourism_data (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID UNIQUE NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    visitor_count_per_year INTEGER,
    ticket_price_eur NUMERIC(8,2),
    accessibility_score NUMERIC(5,2),
    visibility_score NUMERIC(5,2),
    historical_significance NUMERIC(5,2),
    photographic_value NUMERIC(5,2),
    current_condition_score NUMERIC(5,2),
    proximity_to_city_km NUMERIC(8,2),
    nearby_amenities_score NUMERIC(5,2),
    max_daily_visitors INTEGER,
    guided_tour_available BOOLEAN DEFAULT false,
    wheelchair_accessible BOOLEAN DEFAULT false,
    public_transport_access BOOLEAN DEFAULT false,
    peak_season VARCHAR(50),
    tourism_notes TEXT,
    last_updated TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 多水道对比分析
CREATE TABLE IF NOT EXISTS aqueduct_comparison (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    comparison_name VARCHAR(200),
    aqueduct_ids JSONB NOT NULL,
    analysis_time TIMESTAMPTZ NOT NULL,
    structural_metrics JSONB,
    cost_metrics JSONB,
    tourism_metrics JSONB,
    radar_chart_data JSONB,
    priority_ranking JSONB,
    overall_score JSONB,
    recommendation_summary TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_comparison_time ON aqueduct_comparison (analysis_time DESC);

-- 预置水道旅游数据
INSERT INTO aqueduct_tourism_data (aqueduct_id, visitor_count_per_year, ticket_price_eur, accessibility_score, visibility_score, historical_significance, photographic_value, current_condition_score, proximity_to_city_km, nearby_amenities_score, max_daily_visitors, guided_tour_available, wheelchair_accessible, public_transport_access, peak_season)
SELECT a.id, 
    CASE a.name
        WHEN 'Claudia水道' THEN 250000
        WHEN 'Virgo水道' THEN 180000
        WHEN 'Marta水道' THEN 150000
        WHEN 'Appia水道' THEN 120000
        WHEN 'Anio Novus水道' THEN 80000
        WHEN 'Julia水道' THEN 60000
        WHEN 'Traiana水道' THEN 90000
        ELSE 40000
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN 12.0
        WHEN 'Virgo水道' THEN 8.0
        WHEN 'Marta水道' THEN 10.0
        ELSE 5.0
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN 4.5
        WHEN 'Virgo水道' THEN 4.8
        WHEN 'Marta水道' THEN 3.8
        WHEN 'Appia水道' THEN 3.2
        ELSE 3.5
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN 4.9
        WHEN 'Anio Novus水道' THEN 4.7
        WHEN 'Marta水道' THEN 4.6
        WHEN 'Virgo水道' THEN 4.3
        ELSE 4.0
    END,
    CASE a.name
        WHEN 'Appia水道' THEN 5.0
        WHEN 'Claudia水道' THEN 4.8
        WHEN 'Marta水道' THEN 4.7
        WHEN 'Virgo水道' THEN 4.5
        ELSE 4.0
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN 4.8
        WHEN 'Marta水道' THEN 4.7
        WHEN 'Anio Novus水道' THEN 4.5
        WHEN 'Virgo水道' THEN 4.2
        ELSE 3.8
    END,
    CASE a.name
        WHEN 'Virgo水道' THEN 4.7
        WHEN 'Claudia水道' THEN 4.2
        WHEN 'Traiana水道' THEN 4.0
        ELSE 3.5
    END,
    CASE a.name
        WHEN 'Virgo水道' THEN 2.5
        WHEN 'Claudia水道' THEN 5.0
        WHEN 'Appia水道' THEN 3.0
        ELSE 12.0
    END,
    CASE a.name
        WHEN 'Virgo水道' THEN 4.8
        WHEN 'Claudia水道' THEN 4.2
        WHEN 'Appia水道' THEN 4.0
        ELSE 3.0
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN 2000
        WHEN 'Virgo水道' THEN 3000
        WHEN 'Marta水道' THEN 1200
        ELSE 500
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN true
        WHEN 'Virgo水道' THEN true
        ELSE false
    END,
    CASE a.name
        WHEN 'Virgo水道' THEN true
        WHEN 'Claudia水道' THEN true
        ELSE false
    END,
    CASE a.name
        WHEN 'Virgo水道' THEN true
        WHEN 'Claudia水道' THEN true
        WHEN 'Appia水道' THEN true
        ELSE false
    END,
    CASE a.name
        WHEN 'Claudia水道' THEN '4-10月'
        WHEN 'Virgo水道' THEN '全年'
        ELSE '5-9月'
    END
FROM aqueducts a;
