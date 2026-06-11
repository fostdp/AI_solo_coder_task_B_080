-- ============================================
-- Feature扩展: 四大新功能的数据库表
-- ============================================

-- ============================================
-- Feature 1: 古罗马混凝土耐久性反演
-- ============================================

CREATE TABLE IF NOT EXISTS roman_concrete_formulas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    formula_name VARCHAR(200) NOT NULL,
    lime_ratio NUMERIC(8,4) NOT NULL DEFAULT 1.0,
    pozzolana_ratio NUMERIC(8,4) NOT NULL DEFAULT 1.0,
    aggregate_ratio NUMERIC(8,4) NOT NULL DEFAULT 3.0,
    water_ratio NUMERIC(8,4) NOT NULL DEFAULT 0.85,
    aggregate_type VARCHAR(100),
    additive_type VARCHAR(100),
    original_fy_mpa NUMERIC(10,4) NOT NULL,
    original_fm_mpa NUMERIC(10,4),
    original_em_gpa NUMERIC(10,4),
    porosity NUMERIC(8,4),
    pore_size_distribution JSONB,
    durability_index NUMERIC(8,4),
    era_description TEXT,
    archaeological_sources TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO roman_concrete_formulas
(formula_name, lime_ratio, pozzolana_ratio, aggregate_ratio, water_ratio, aggregate_type, additive_type,
 original_fy_mpa, original_fm_mpa, original_em_gpa, porosity, durability_index, era_description, archaeological_sources)
VALUES
('标准罗马混凝土配方 (Opus Caementicium)', 1.0, 1.2, 3.5, 0.85, '石灰华/火山砾', '无',
 8.5, 1.8, 25.0, 0.28, 0.85, '罗马帝国时期 (公元前1世纪 - 公元3世纪)', 'Vitruvius De Architectura, Book 2; Pompeii遗址'),
('高强度火山灰砂浆 (Puteolanus Pulvis)', 0.85, 1.6, 3.2, 0.78, 'Pozzuoli火山灰', '无',
 10.5, 2.2, 28.0, 0.24, 0.90, '罗马共和国末期-帝国早期', 'Pliny the Elder, Naturalis Historia XXXIII; Bay of Naples遗址'),
('水下耐水配方 (Opus Signinum)', 1.1, 1.0, 4.0, 0.90, '碎砖骨料 (Cocciopesto)', '无',
 7.5, 1.5, 22.0, 0.30, 0.88, '罗马共和国时期', 'Ostia Antica海港结构, 水下建筑遗迹'),
('拱券专用砂浆 (Arcus Mortar)', 1.2, 0.8, 3.8, 0.88, '细骨料石灰华', '动物脂肪微量',
 7.0, 1.4, 20.0, 0.32, 0.80, '帝国鼎盛期 (公元1-2世纪)', 'Pantheon穹顶, Colosseum拱券结构'),
('石灰华骨料结构混凝土', 0.95, 1.1, 4.2, 0.82, 'Tivoli石灰华', '无',
 9.0, 2.0, 26.0, 0.26, 0.86, '克劳狄王朝工程时期', 'Aqua Claudia拱券, Domus Aurea遗址'),
('Claudia水道专用高强度配方', 0.90, 1.4, 3.6, 0.80, '混合骨料', '火山岩粉',
 9.8, 2.1, 27.0, 0.25, 0.89, '公元52年 克劳狄皇帝敕令工程', 'Aqua Claudia考古取样分析报告'),
('Virgo水道保水性配方', 1.05, 1.0, 3.3, 0.83, 'Travertine细骨料', '橄榄油残渣',
 8.0, 1.7, 24.0, 0.27, 0.87, '公元前19年 Agrippa监造', 'Aqua Virgo至今仍在使用, 结构完整性极佳'),
('Anio Novus水道长寿命配方', 0.88, 1.5, 3.4, 0.79, 'Anio河河砾', '无',
 10.2, 2.15, 27.5, 0.23, 0.91, '公元38年 Caligula开工-Claudius完工', 'Aqua Anio Novus桥墩, 历经2000年风化'),
('海工水下混凝土 (Harenae)', 1.15, 1.3, 3.9, 0.92, '火山砂', '海水拌合',
 8.8, 1.9, 23.5, 0.29, 0.92, '公元前2世纪-帝国时期', 'Portus/Centocelle海港, 海中2000年未坏'),
('基础大体积混凝土', 1.20, 0.9, 4.5, 0.95, '大骨料 (caementa)', '无',
 6.5, 1.3, 18.0, 0.34, 0.82, '从罗马王政时期到帝国', '各类建筑物基础层, 庞贝, 罗马广场'),
('饰面抹灰砂浆 (Opus Albarium)', 1.4, 0.6, 2.5, 0.80, '大理石粉', '石灰乳液',
 6.0, 1.2, 22.0, 0.30, 0.78, '共和国晚期-帝国鼎盛期', 'Vitruvius VII, Villa of the Mysteries壁画基层')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS concrete_inversion_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES structure_segments(id) ON DELETE SET NULL,
    analysis_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    observed_weathering_depth NUMERIC(12,4) NOT NULL,
    observed_strength NUMERIC(12,4) NOT NULL,
    observed_mortar_ph NUMERIC(8,4),
    age_years NUMERIC(12,2),
    best_match_formula_id UUID REFERENCES roman_concrete_formulas(id),
    candidate_formulas JSONB,
    inversion_confidence NUMERIC(8,4),
    inferred_original_fy NUMERIC(12,4),
    inferred_durability_mechanism JSONB,
    leaching_rate NUMERIC(12,6),
    carbonation_depth NUMERIC(10,4),
    modern_reference_formula JSONB,
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_inv_aqueduct ON concrete_inversion_results(aqueduct_id);
CREATE INDEX IF NOT EXISTS idx_inv_segment ON concrete_inversion_results(segment_id);

-- ============================================
-- Feature 2: 地震易损性评估
-- ============================================

CREATE TABLE IF NOT EXISTS historical_earthquakes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_name VARCHAR(200) NOT NULL,
    event_date VARCHAR(30),
    magnitude NUMERIC(6,3) NOT NULL,
    epicenter_lat NUMERIC(10,6),
    epicenter_lng NUMERIC(10,6),
    depth_km NUMERIC(8,3),
    intensity_msk NUMERIC(5,2),
    region VARCHAR(100),
    affected_aqueducts JSONB,
    historical_sources TEXT,
    damage_description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO historical_earthquakes
(event_name, event_date, magnitude, epicenter_lat, epicenter_lng, depth_km, intensity_msk, region,
 affected_aqueducts, historical_sources, damage_description)
VALUES
('公元512年大地震 (Venetia-Ancona)', '512-06-08', 6.9, 42.1000, 12.8000, 12, 9.0, 'Latium北部',
 '{"aqueducts":["Aqua Claudia","Aqua Anio Novus"],"damage_count":3}', 'Cassiodorus Variae; Liber Pontificalis', 'Aqua Claudia水道北部拱券群多处坍塌，桥墩倾斜3度'),
('公元801年罗马地震 (Anno Domini 801)', '801-04-29', 6.5, 41.9000, 12.5000, 8, 8.0, 'Roma Urbs',
 '{"aqueducts":["Aqua Anio Novus","Aqua Marcia"],"damage_count":2}', 'Annales Laurissenses; 罗马教会编年史', 'Anio Novus水道3处桥墩产生不均匀沉降，拱券开裂'),
('1349年Latium地震群 (Terremoto del Lazio)', '1349-09-09', 6.7, 41.7000, 13.0000, 10, 9.0, 'Latium东南',
 '{"aqueducts":["Aqua Claudia","Aqua Marcia","Aqua Tepula"],"damage_count":4}', 'Petrarca书信; 梵蒂冈档案', '多处拱券结构彻底损毁，部分水道此后废弃不用'),
('1695年Abruzzo地震 (Terremoto d''Abruzzo)', '1695-01-14', 6.2, 42.4000, 13.2000, 15, 8.0, 'Abruzzo边界',
 '{"aqueducts":["Aqua Marcia","Aqua Julia"],"damage_count":2}', 'INGV Catalogo Parametrico', 'Aqua Marcia风化加速，多处砂浆表面剥落'),
('1915年Avezzano地震 (Terremoto della Marsica)', '1915-01-13', 7.0, 42.0000, 13.4000, 15, 10.0, 'Fucino盆地',
 '{"aqueducts":["Aqua Claudia","Aqua Anio Novus","Aqua Marcia","Aqua Julia"],"damage_count":6}', 'INGV; 意大利皇家地质调查局', '多条水道监测到沉降异常，拱顶位移超15mm，整体虽未坍塌但需紧急评估'),
('2016年Amatrice-Visso地震序列', '2016-08-24', 6.0, 42.7000, 13.2000, 8, 7.0, 'Central Apennines',
 '{"aqueducts":["Aqua Anio Vetus","Aqua Marcia"],"damage_count":2}', 'INGV Bollettino Sismico', '结构监测传感器数据异常，应力读数瞬时升高35%')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS aqueduct_seismic_risks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE UNIQUE,
    region VARCHAR(100),
    peak_ground_accel_475yr NUMERIC(10,6),
    peak_ground_accel_2475yr NUMERIC(10,6),
    overall_risk_level VARCHAR(20),
    site_class VARCHAR(5),
    soil_amplification NUMERIC(8,4),
    predominant_period_sec NUMERIC(8,4),
    vulnerable_segments INTEGER,
    estimated_total_loss NUMERIC(15,2),
    analysis_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS seismic_vulnerabilities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE,
    segment_id UUID REFERENCES structure_segments(id) ON DELETE SET NULL,
    analysis_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    damage_state VARCHAR(30) NOT NULL,
    magnitude NUMERIC(6,3),
    pga_g NUMERIC(10,6),
    probability NUMERIC(8,4),
    fragility_curve_params JSONB,
    capacity_spectrum JSONB,
    demand_spectrum JSONB,
    expected_repair_cost NUMERIC(15,2),
    expected_downtime_days INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_seisvul_aqueduct ON seismic_vulnerabilities(aqueduct_id);
CREATE INDEX IF NOT EXISTS idx_seisvul_segment ON seismic_vulnerabilities(segment_id);

-- ============================================
-- Feature 3: 修复材料长期性能预测
-- ============================================

CREATE TABLE IF NOT EXISTS accelerated_aging_data (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    material_id UUID NOT NULL REFERENCES repair_materials(id) ON DELETE CASCADE,
    test_type VARCHAR(50) NOT NULL,
    temperature_c NUMERIC(8,3) NOT NULL,
    humidity_pct NUMERIC(8,3),
    cycles INTEGER DEFAULT 0,
    exposure_days INTEGER NOT NULL,
    strength_retention NUMERIC(8,4) NOT NULL,
    mass_loss_pct NUMERIC(10,4),
    elastic_modulus_loss NUMERIC(8,4),
    cracking_index NUMERIC(8,4),
    ph_change NUMERIC(6,3),
    test_notes TEXT,
    test_standard VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_aging_material ON accelerated_aging_data(material_id);

CREATE TABLE IF NOT EXISTS material_lifetime_predictions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    material_id UUID NOT NULL REFERENCES repair_materials(id) ON DELETE CASCADE,
    prediction_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scenario VARCHAR(50) NOT NULL,
    prediction_years INTEGER NOT NULL,
    arrhenius_activation_ev NUMERIC(10,6),
    time_temp_shift_factor JSONB,
    degradation_curve JSONB,
    strength_at_50yr NUMERIC(8,4),
    strength_at_100yr NUMERIC(8,4),
    estimated_service_life NUMERIC(10,2),
    threshold_strength_ratio NUMERIC(8,4),
    confidence_interval_low NUMERIC(8,4),
    confidence_interval_high NUMERIC(8,4),
    model_assumptions TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_life_material ON material_lifetime_predictions(material_id);

-- ============================================
-- Feature 4: 多水道对比与旅游规划
-- ============================================

CREATE TABLE IF NOT EXISTS aqueduct_tourism_data (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    aqueduct_id UUID NOT NULL REFERENCES aqueducts(id) ON DELETE CASCADE UNIQUE,
    visitor_count_per_year INTEGER DEFAULT 0,
    ticket_price_eur NUMERIC(10,2),
    accessibility_score NUMERIC(8,4),
    visibility_score NUMERIC(8,4),
    historical_significance NUMERIC(8,4),
    photographic_value NUMERIC(8,4),
    current_condition_score NUMERIC(8,4),
    proximity_to_city_km NUMERIC(10,3),
    nearby_amenities_score NUMERIC(8,4),
    max_daily_visitors INTEGER,
    guided_tour_available BOOLEAN DEFAULT false,
    wheelchair_accessible BOOLEAN DEFAULT false,
    public_transport_access BOOLEAN DEFAULT false,
    peak_season VARCHAR(30),
    tourism_notes TEXT,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO aqueduct_tourism_data
(aqueduct_id, visitor_count_per_year, ticket_price_eur, accessibility_score, visibility_score,
 historical_significance, photographic_value, current_condition_score, proximity_to_city_km,
 nearby_amenities_score, max_daily_visitors, guided_tour_available, wheelchair_accessible,
 public_transport_access, peak_season, tourism_notes)
SELECT a.id,
    CASE a.name
        WHEN 'Appia水道' THEN 95000 WHEN 'Anio Vetus水道' THEN 45000
        WHEN 'Marta水道' THEN 210000 WHEN 'Tepula水道' THEN 30000
        WHEN 'Julia水道' THEN 55000 WHEN 'Virgo水道' THEN 850000
        WHEN 'Alsietina水道' THEN 25000 WHEN 'Claudia水道' THEN 320000
        WHEN 'Anio Novus水道' THEN 65000 WHEN 'Traiana水道' THEN 75000
        WHEN 'Severiana水道' THEN 35000 ELSE 50000 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 15.0 WHEN 'Marta水道' THEN 12.0
        WHEN 'Claudia水道' THEN 10.0 ELSE 8.0 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 0.92 WHEN 'Marta水道' THEN 0.85 WHEN 'Claudia水道' THEN 0.78
        WHEN 'Appia水道' THEN 0.68 ELSE 0.55 END,
    CASE a.name
        WHEN 'Claudia水道' THEN 0.95 WHEN 'Marta水道' THEN 0.90 WHEN 'Virgo水道' THEN 0.70
        ELSE 0.60 END,
    CASE a.name
        WHEN 'Appia水道' THEN 0.98 WHEN 'Virgo水道' THEN 0.95 WHEN 'Marta水道' THEN 0.90
        WHEN 'Claudia水道' THEN 0.88 ELSE 0.70 END,
    CASE a.name
        WHEN 'Claudia水道' THEN 0.95 WHEN 'Marta水道' THEN 0.88 WHEN 'Anio Novus水道' THEN 0.80
        ELSE 0.65 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 0.92 WHEN 'Claudia水道' THEN 0.68 WHEN 'Marta水道' THEN 0.60
        ELSE 0.50 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 1.5 WHEN 'Appia水道' THEN 5.0 WHEN 'Marta水道' THEN 8.0
        WHEN 'Claudia水道' THEN 12.0 ELSE 15.0 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 0.95 WHEN 'Appia水道' THEN 0.80 WHEN 'Marta水道' THEN 0.50
        ELSE 0.40 END,
    CASE a.name
        WHEN 'Virgo水道' THEN 3200 WHEN 'Marta水道' THEN 2100 WHEN 'Claudia水道' THEN 1800
        ELSE 500 END,
    true,
    CASE a.name WHEN 'Virgo水道' THEN true WHEN 'Marta水道' THEN true ELSE false END,
    CASE a.name WHEN 'Virgo水道' THEN true WHEN 'Appia水道' THEN true ELSE false END,
    CASE a.name WHEN 'Virgo水道' THEN '全年(高峰4-10月)' WHEN 'Marta水道' THEN '4-10月' ELSE '4-9月' END,
    '基于考古记录与现代旅游数据综合的初始值'
FROM aqueducts a
ON CONFLICT (aqueduct_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS aqueduct_comparisons (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    comparison_name VARCHAR(200),
    aqueduct_ids JSONB NOT NULL,
    analysis_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    structural_metrics JSONB,
    cost_metrics JSONB,
    tourism_metrics JSONB,
    radar_chart_data JSONB,
    priority_ranking JSONB,
    overall_score JSONB,
    recommendation_summary TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
