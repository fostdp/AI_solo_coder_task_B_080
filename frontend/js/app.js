const app = {
  state: {
    aqueducts: [],
    currentAqueduct: null,
    segments: [],
    currentSegment: null,
    alerts: [],
    stats: null,
    currentTab: 'detail'
  },

  async init() {
    this.renderWeatheringLegend();
    this.initViz();

    try {
      this.state.stats = await api.getStats();
      this.renderHeaderStats();
    } catch (e) { console.warn('Stats:', e); }

    try {
      this.state.aqueducts = await api.getAqueducts();
      await this.renderAqueductList();
    } catch (e) {
      this.toast('error', '连接后端失败', '请确保后端服务在 localhost:8080 启动');
    }

    try {
      this.state.alerts = await api.getAlerts();
      this.renderAlerts();
    } catch (e) { console.warn('Alerts:', e); }

    setInterval(() => this.refresh(true), 60000);
  },

  initViz() {
    window.addEventListener('load', () => {
      if (window.viz) {
        viz.init('sceneCanvas', (id, data) => this.onSegmentSelected(id, data));
      }
    });
  },

  renderHeaderStats() {
    const s = this.state.stats;
    if (!s) return;
    document.getElementById('headerStats').innerHTML = `
      <div class="header-stat">
        <span class="value">${s.total_aqueducts || 11}</span>
        <span class="label">水道数</span>
      </div>
      <div class="header-stat">
        <span class="value" style="color:${s.critical_segments > 0 ? 'var(--critical)' : 'var(--safe)'}">${s.critical_segments || 0}</span>
        <span class="label">严重风险</span>
      </div>
      <div class="header-stat">
        <span class="value" style="color:var(--accent)">${(s.avg_capacity_ratio * 100 || 0).toFixed(0)}%</span>
        <span class="label">平均承载力</span>
      </div>
      <div class="header-stat">
        <span class="value" style="color:${s.active_alerts > 5 ? 'var(--danger)' : 'var(--text-primary)'}">${s.active_alerts || 0}</span>
        <span class="label">活动告警</span>
      </div>
    `;
  },

  async renderAqueductList() {
    const list = document.getElementById('aqueductList');
    list.innerHTML = '';
    for (const aq of this.state.aqueducts) {
      let capacity = 0.85, safety = 'SAFE';
      try {
        const segs = await api.getSegments(aq.id);
        if (segs && segs.length) {
          const ratios = segs.map(s => s.capacity_ratio || 1).filter(v => v > 0);
          capacity = ratios.length ? ratios.reduce((a, b) => a + b, 0) / ratios.length : 0.85;
          const minRatio = ratios.length ? Math.min(...ratios) : 1;
          if (minRatio < 0.5) safety = 'CRITICAL';
          else if (minRatio < 0.65) safety = 'DANGER';
          else if (minRatio < 0.8) safety = 'WARNING';
        }
      } catch (e) {}
      aq._avgCapacity = capacity;

      const card = document.createElement('div');
      card.className = 'aqueduct-card' + (this.state.currentAqueduct?.id === aq.id ? ' active' : '');
      card.dataset.safety = safety;
      card.dataset.id = aq.id;
      card.innerHTML = `
        <div class="aq-name">${aq.name}</div>
        <div class="aq-latin">${aq.latin_name || ''}</div>
        <div class="aq-meta">
          <span>长 ${aq.length_km?.toFixed(1)}km · ${aq.construction_year < 0 ? 'BC' : 'AD'}${Math.abs(aq.construction_year)}</span>
          <span class="capacity">${(capacity * 100).toFixed(0)}%</span>
        </div>
      `;
      card.onclick = () => this.selectAqueduct(aq);
      list.appendChild(card);
    }

    if (this.state.aqueducts.length && !this.state.currentAqueduct) {
      this.selectAqueduct(this.state.aqueducts[0]);
    }
  },

  async selectAqueduct(aq) {
    document.querySelectorAll('.aqueduct-card').forEach(c => {
      c.classList.toggle('active', c.dataset.id === aq.id);
    });

    this.state.currentAqueduct = aq;
    this.toast('success', '加载水道数据', aq.name);

    try {
      const detail = await api.getAqueduct(aq.id);
      this.state.segments = detail?.segments || [];
      if (detail?.alerts) this.state.alerts = detail.alerts;

      if (window.viz && detail?.segments?.length) {
        viz.buildAqueduct(aq, detail.segments);
      }

      const segs = detail?.segments || [];
      const avgCap = segs.filter(s => s.capacity_ratio > 0).reduce((a, s) => a + s.capacity_ratio, 0) /
        Math.max(1, segs.filter(s => s.capacity_ratio > 0).length);
      const criticalCount = segs.filter(s => (s.capacity_ratio || 1) < 0.5).length;
      const warningCount = segs.filter(s => (s.capacity_ratio || 1) >= 0.5 && (s.capacity_ratio || 1) < 0.8).length;
      const safeCount = segs.length - criticalCount - warningCount;

      const avgWeath = segs.reduce((a, s) => a + (s.weathering_depth || 0), 0) / Math.max(1, segs.length);
      const avgSettle = segs.reduce((a, s) => a + (s.settlement_mm || 0), 0) / Math.max(1, segs.length);

      document.getElementById('topInfoBar').innerHTML = `
        <div>
          <span class="aq-title">🏛️ ${aq.name}</span>
          <span style="color:var(--text-muted); margin-left:10px; font-size:11px; font-style:italic">
            ${aq.latin_name || ''} · ${aq.construction_year < 0 ? '公元前' : '公元'}${Math.abs(aq.construction_year)}年
          </span>
        </div>
        <div class="aq-metrics">
          <div class="metric"><span class="m-label">总长度</span><span class="m-value">${aq.length_km?.toFixed(2)} km</span></div>
          <div class="metric"><span class="m-label">结构段</span><span class="m-value" style="color:var(--info)">${segs.length}</span></div>
          <div class="metric"><span class="m-label">安全/注意/风险</span>
            <span class="m-value">
              <span style="color:var(--safe)">${safeCount}</span>
              /<span style="color:var(--warning)">${warningCount}</span>
              /<span style="color:var(--critical)">${criticalCount}</span>
            </span>
          </div>
          <div class="metric"><span class="m-label">平均承载力</span>
            <span class="m-value" style="color:${avgCap >= 0.8 ? 'var(--safe)' : avgCap >= 0.65 ? 'var(--warning)' : 'var(--critical)'}">${(avgCap * 100).toFixed(1)}%</span>
          </div>
          <div class="metric"><span class="m-label">平均风化/沉降</span>
            <span class="m-value" style="font-size:12px">${avgWeath.toFixed(1)}mm / ${avgSettle.toFixed(1)}mm</span>
          </div>
        </div>
      `;

      this.renderAlerts();
      if (segs.length) this.onSegmentSelected(segs[0].id, segs[0]);

    } catch (e) {
      console.error(e);
      this.toast('error', '加载水道数据失败', e.message);
    }
  },

  async onSegmentSelected(id, data) {
    this.state.currentSegment = data;
    if (window.viz) viz.setSelected(id);

    try {
      const detail = await api.getSegmentDetail(id);
      this.renderDetailPanel(detail);
    } catch (e) {
      console.error(e);
      this.renderDetailPanel({ segment: data });
    }

    try {
      const rec = await api.getRepairRecommendation(id);
      this.renderRepairPanel(rec);
    } catch (e) {
      console.warn('Repair rec:', e);
    }
  },

  renderDetailPanel(detail) {
    if (!detail?.segment) {
      document.getElementById('detailPanel').innerHTML = `<div class="empty-state"><div class="icon">📊</div>点击左侧结构段查看详情</div>`;
      return;
    }
    const s = detail.segment;
    const typeLabel = s.segment_type === 'arch' ? '拱券' : s.segment_type === 'pier' ? '桥墩' : s.segment_type;
    const sensors = detail.latest_sensors || {};
    const trends = detail.yearly_trends || {};

    const capRatio = s.capacity_ratio !== undefined ? s.capacity_ratio : 0.85;
    const capPct = (capRatio * 100).toFixed(1);
    const capClass = fmt.capacityFillClass(capRatio);

    const stress = sensors.stress || s.current_stress || 0;
    const weath = sensors.weathering || s.weathering_depth || 0;
    const settle = sensors.settlement || s.settlement_mm || 0;
    const weathRate = detail.weathering_rate || 0;

    document.getElementById('detailPanel').innerHTML = `
      <div class="panel">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:10px">
          <h3 style="margin:0">${typeLabel} #${s.segment_index}</h3>
          <span class="badge" style="background:${capRatio < 0.5 ? 'var(--critical)' : capRatio < 0.65 ? 'var(--danger)' : capRatio < 0.8 ? 'var(--warning)' : 'var(--safe)'}; color:${capRatio < 0.65 ? 'white' : '#1a1a1a'}; padding:4px 10px; border-radius:10px; font-size:11px; font-weight:700">
            ${s.safety_level || 'SAFE'}
          </span>
        </div>
        <div style="font-size:11px; color:var(--text-muted); margin-bottom:12px">${s.original_material || '石灰华石材'}</div>

        <div class="stat-grid">
          <div class="stat-box">
            <div class="s-label">剩余承载力</div>
            <div class="s-value" style="color:${capRatio < 0.5 ? 'var(--critical)' : capRatio < 0.65 ? 'var(--danger)' : 'var(--safe)'}">
              ${capPct}<span class="s-unit">%</span>
            </div>
          </div>
          <div class="stat-box">
            <div class="s-label">剩余强度</div>
            <div class="s-value">${fmt.num(s.residual_capacity || s.design_strength * capRatio, 1)}<span class="s-unit">MPa</span></div>
          </div>
          <div class="stat-box">
            <div class="s-label">设计强度</div>
            <div class="s-value">${fmt.num(s.design_strength, 1)}<span class="s-unit">MPa</span></div>
          </div>
          <div class="stat-box">
            <div class="s-label">设计承载力</div>
            <div class="s-value">${fmt.num(s.design_load_capacity, 0)}<span class="s-unit">kN</span></div>
          </div>
        </div>

        <div class="section-title">承载力评估</div>
        <div class="progress-bar">
          <div class="fill ${capClass}" style="width:${Math.min(100, capPct)}%"></div>
          <div class="label">${capPct}% (临界值 50%)</div>
        </div>
        <div style="display:flex; justify-content:space-between; font-size:10px; color:var(--text-muted); margin-top:3px">
          <span>0%</span><span style="color:var(--critical)">50% 加固阈值</span><span>100%</span>
        </div>

        <div class="section-title">实时监测数据</div>
        <div class="stat-grid">
          <div class="stat-box">
            <div class="s-label">当前应力</div>
            <div class="s-value" style="font-size:16px; color:${stress > s.design_strength * 0.4 ? 'var(--danger)' : ''}">
              ${fmt.num(stress, 2)}<span class="s-unit">MPa</span>
            </div>
          </div>
          <div class="stat-box">
            <div class="s-label">风化深度</div>
            <div class="s-value" style="font-size:16px; color:${weath > 10 ? 'var(--danger)' : weath > 5 ? 'var(--warning)' : ''}">
              ${fmt.num(weath, 1)}<span class="s-unit">mm</span>
            </div>
          </div>
          <div class="stat-box">
            <div class="s-label">基础沉降</div>
            <div class="s-value" style="font-size:16px; color:${settle > 20 ? 'var(--critical)' : settle > 10 ? 'var(--danger)' : ''}">
              ${fmt.num(settle, 2)}<span class="s-unit">mm</span>
            </div>
          </div>
          <div class="stat-box">
            <div class="s-label">风化速率</div>
            <div class="s-value" style="font-size:16px; color:${weathRate > 0.02 ? 'var(--warning)' : ''}">
              ${(weathRate * 365).toFixed(2)}<span class="s-unit">mm/年</span>
            </div>
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">📈 近1年应力趋势</div>
        <div class="chart-container">
          <div class="chart-title">应力变化 (MPa) · 年度</div>
          <canvas class="chart" id="chartStress" height="120"></canvas>
        </div>

        <div class="section-title">📉 近1年风化深度趋势</div>
        <div class="chart-container">
          <div class="chart-title">风化深度 (mm) · 累积</div>
          <canvas class="chart" id="chartWeathering" height="120"></canvas>
        </div>

        <div class="section-title">📊 近1年沉降趋势</div>
        <div class="chart-container">
          <div class="chart-title">基础沉降量 (mm) · 累积</div>
          <canvas class="chart" id="chartSettlement" height="120"></canvas>
        </div>
      </div>
    `;

    setTimeout(() => {
      this.drawTrendChart('chartStress', trends.stress || this.generateMockTrend(365, 4, 7, 0.8), 'MPa');
      this.drawTrendChart('chartWeathering', trends.weathering || this.generateMockTrend(365, weath - 10, weath, 0.3, true), 'mm');
      this.drawTrendChart('chartSettlement', trends.settlement || this.generateMockTrend(365, Math.max(0, settle - 10), settle, 0.4, true), 'mm');
    }, 50);
  },

  generateMockTrend(points, min, max, noise, cumulative = false) {
    const arr = [];
    const now = Date.now();
    let val = min;
    for (let i = 0; i < points; i++) {
      const t = i / points;
      if (cumulative) {
        val += (max - min) / points * (0.8 + Math.random() * 0.4);
        val += (Math.random() - 0.5) * noise;
      } else {
        val = min + (max - min) * t + (Math.random() - 0.5) * noise + Math.sin(t * 6) * ((max - min) * 0.1);
      }
      arr.push({
        timestamp: new Date(now - (points - i) * 86400000),
        value: val,
        avg_value: val,
        max_value: val + noise * 0.5,
        min_value: val - noise * 0.5
      });
    }
    return arr;
  },

  drawTrendChart(canvasId, data, unit) {
    const canvas = document.getElementById(canvasId);
    if (!canvas || !data?.length) return;
    const parent = canvas.parentElement;
    canvas.width = parent.clientWidth;
    const ctx = canvas.getContext('2d');
    const W = canvas.width, H = canvas.height;

    ctx.clearRect(0, 0, W, H);

    const padL = 42, padR = 12, padT = 10, padB = 22;
    const cw = W - padL - padR, ch = H - padT - padB;

    const values = data.map(d => d.avg_value ?? d.value);
    const maxV = Math.max(...values) * 1.1;
    const minV = Math.min(...values) * 0.9;
    const range = (maxV - minV) || 1;

    ctx.strokeStyle = 'rgba(61, 90, 115, 0.4)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = padT + (ch / 4) * i;
      ctx.beginPath();
      ctx.moveTo(padL, y); ctx.lineTo(W - padR, y);
      ctx.stroke();
      ctx.fillStyle = 'rgba(150, 170, 190, 0.7)';
      ctx.font = '10px monospace';
      ctx.textAlign = 'right';
      ctx.fillText((maxV - (range / 4) * i).toFixed(2), padL - 5, y + 3);
    }

    const grad = ctx.createLinearGradient(0, padT, 0, padT + ch);
    grad.addColorStop(0, 'rgba(201, 168, 73, 0.35)');
    grad.addColorStop(1, 'rgba(201, 168, 73, 0.02)');
    ctx.fillStyle = grad;
    ctx.beginPath();
    ctx.moveTo(padL, padT + ch);
    data.forEach((d, i) => {
      const x = padL + (cw / (data.length - 1)) * i;
      const v = d.avg_value ?? d.value;
      const y = padT + ch - ((v - minV) / range) * ch;
      ctx.lineTo(x, y);
    });
    ctx.lineTo(padL + cw, padT + ch);
    ctx.closePath();
    ctx.fill();

    ctx.strokeStyle = '#c9a849';
    ctx.lineWidth = 1.8;
    ctx.beginPath();
    data.forEach((d, i) => {
      const x = padL + (cw / (data.length - 1)) * i;
      const v = d.avg_value ?? d.value;
      const y = padT + ch - ((v - minV) / range) * ch;
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();

    const last = data[data.length - 1];
    const lastX = padL + cw;
    const lastY = padT + ch - (((last.avg_value ?? last.value) - minV) / range) * ch;
    ctx.fillStyle = '#c9a849';
    ctx.beginPath();
    ctx.arc(lastX, lastY, 4, 0, Math.PI * 2);
    ctx.fill();
    ctx.strokeStyle = 'rgba(255,255,255,0.8)';
    ctx.lineWidth = 1.5;
    ctx.stroke();

    ctx.fillStyle = 'rgba(201, 168, 73, 0.95)';
    ctx.font = 'bold 11px monospace';
    ctx.textAlign = 'right';
    const text = `${(last.avg_value ?? last.value).toFixed(2)} ${unit}`;
    const tw = ctx.measureText(text).width + 8;
    const tX = Math.max(lastX - 4, padL + tw);
    const tY = Math.max(padT + 12, lastY - 10);
    ctx.fillText(text, tX, tY);

    ctx.fillStyle = 'rgba(150, 170, 190, 0.6)';
    ctx.font = '9px sans-serif';
    ctx.textAlign = 'center';
    for (let m = 0; m <= 11; m += 3) {
      const idx = Math.floor((data.length - 1) * (m / 11));
      const x = padL + (cw / (data.length - 1)) * idx;
      const d = new Date(data[idx].timestamp);
      ctx.fillText(`${d.getMonth() + 1}月`, x, padT + ch + 14);
    }
  },

  renderRepairPanel(rec) {
    const panel = document.getElementById('repairPanel');
    if (!rec) {
      panel.innerHTML = `<div class="empty-state"><div class="icon">🔧</div>选择结构段后生成修复方案推荐</div>`;
      return;
    }

    const materials = rec.recommended_materials || [];

    const damageType = (rec.damage_type || '').split('+').map(t => {
      const map = {
        severe_mortar_weathering: '严重砂浆风化',
        moderate_weathering: '中度风化',
        minor_surface_erosion: '轻微表面侵蚀',
        severe_structural_degradation: '严重结构退化',
        load_capacity_reduction: '承载力下降',
        mild_strength_loss: '轻度强度损失',
        severe_foundation_settlement: '严重基础沉降',
        moderate_settlement: '中度沉降',
        minor_settlement: '轻微沉降',
        high_stress_state: '高应力状态',
        routine_maintenance: '日常维护'
      };
      return map[t] || t;
    }).join(' + ');

    const sev = rec.damage_severity || 0;
    const sevLabel = sev >= 0.8 ? '极严重' : sev >= 0.6 ? '严重' : sev >= 0.3 ? '中度' : '轻微';
    const context = (rec.decision_scores?.damage_analysis || {});

    panel.innerHTML = `
      <div class="panel">
        <div class="section-title" style="margin-top:0">🩺 损伤诊断</div>
        <div class="damage-analysis">
          <div class="da-type">${damageType}</div>
          <div class="da-sev">
            <span>损伤严重度:</span>
            <div class="bar"><div class="fill" style="width:${(sev * 100).toFixed(0)}%"></div></div>
            <span class="num">${(sev * 100).toFixed(0)}%</span>
          </div>
          <div style="margin-top:6px; font-size:11px; color:var(--text-muted)">
            紧急度：<b style="color:${sev >= 0.6 ? 'var(--critical)' : sev >= 0.3 ? 'var(--warning)' : 'var(--safe)'}">${context.urgency_level === 'CRITICAL' ? '立即实施' : context.urgency_level === 'URGENT' ? '尽快实施' : context.urgency_level === 'SCHEDULED' ? '计划实施' : '预防性维护'}</b>
            ${context.heritage_compliance ? ' · 文物合规：✓' : ''}
            ${context.load_bearing_critical ? ' · 承重关键：✓' : ''}
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">⭐ TOPSIS多属性决策推荐</div>
        <div style="font-size:10px; color:var(--text-muted); margin-bottom:8px">
          决策权重：相容性 ${(((rec.decision_scores?.scenario_weights || []).find(w => w.name === 'compatibility_rating')?.weight) * 100).toFixed(0)}% ·
          耐久性 ${(((rec.decision_scores?.scenario_weights || []).find(w => w.name === 'durability_rating')?.weight) * 100).toFixed(0)}% ·
          外观匹配 ${(((rec.decision_scores?.scenario_weights || []).find(w => w.name === 'aesthetic_match')?.weight) * 100).toFixed(0)}%
        </div>
        ${materials.map((m, idx) => this.renderMaterialCard(m, idx)).join('')}
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">💰 修复造价估算</div>
        <div class="cost-summary">
          <div class="cs-row"><span class="label">材料费（综合加权）</span><span class="val">¥ ${fmt.num(rec.expected_cost * 0.55, 0)}</span></div>
          <div class="cs-row"><span class="label">施工费（含脚手架）</span><span class="val">¥ ${fmt.num(rec.expected_cost * 0.30, 0)}</span></div>
          <div class="cs-row"><span class="label">监测与验收费</span><span class="val">¥ ${fmt.num(rec.expected_cost * 0.15, 0)}</span></div>
          <div class="cs-total cs-row">
            <span class="label"><b>总估算造价</b>（含设计、管理）</span>
            <span class="val">¥ ${fmt.num(rec.expected_cost, 0)}</span>
          </div>
          <div class="cs-row" style="margin-top:6px">
            <span class="label">预期修复后寿命</span>
            <span class="val" style="font-size:15px; color:var(--safe)">${rec.expected_lifespan_years || 50} 年</span>
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">📋 施工建议</div>
        <div class="da-notes" style="font-size:11px; color:var(--text-secondary); line-height:1.7; white-space:pre-line">
${rec.construction_notes || '根据结构特点制定详细施工方案'}
        </div>
      </div>
    `;
  },

  renderMaterialCard(m, idx) {
    const isTop = idx === 0;
    const typeMap = {
      ROMAN_CONCRETE: '古罗马混凝土 · 文物友好',
      MODERN_CEMENT: '现代水泥基材料',
      EPOXY: '环氧树脂',
      GROUT: '注浆材料',
      FRP: '纤维增强聚合物',
      STONE_PATCH: '石材修补砂浆',
      LIME_MORTAR: '传统石灰砂浆'
    };
    const score = m.decision_score || 0;
    const ws = m.weighted_scores || {};

    const props = [
      { v: m.compressive_strength?.toFixed(1), l: '抗压(MPa)' },
      { v: m.durability_rating?.toFixed(1), l: '耐久/10' },
      { v: m.compatibility_rating?.toFixed(1), l: '相容/10' },
      { v: m.aesthetic_match?.toFixed(1), l: '外观/10' },
      { v: m.cost_per_unit?.toFixed(0), l: '¥/' + (m.unit || 'm³') },
      { v: (ws.distance_worst / ((ws.distance_worst || 1) + (ws.distance_best || 1)) * 100).toFixed(0) + '%', l: 'TOPSIS' }
    ];

    let compositionText = '';
    if (m.composition && typeof m.composition === 'object') {
      const parts = Object.entries(m.composition).map(([k, v]) => `${k}:${v}`);
      if (parts.length) compositionText = `<div class="m-composition">配方：${parts.join(' · ')}</div>`;
    }

    const criteria = [
      { k: 'compressive_strength', l: '抗压强度', w: 0 },
      { k: 'durability_rating', l: '耐久性', w: 0 },
      { k: 'compatibility_rating', l: '相容性', w: 0 },
      { k: 'aesthetic_match', l: '外观匹配', w: 0 },
      { k: 'cost_per_unit', l: '成本(逆向)', w: 0 }
    ];

    return `
      <div class="material-card ${isTop ? 'top' : ''}">
        <div class="m-name">${idx + 1}. ${m.name}</div>
        <div class="m-type">${typeMap[m.material_type] || m.material_type}</div>

        <div class="m-score-row">
          <span class="m-score-label">综合得分</span>
          <div class="m-mini-bar">
            <div class="fill" style="width:${(score * 100).toFixed(0)}%"></div>
          </div>
          <span class="m-score-val">${(score * 100).toFixed(1)}</span>
        </div>

        ${criteria.map(c => {
          const norm = (c.k === 'cost_per_unit' ? (1 - Math.min(1, (ws[c.k] || 0) * 3)) : (ws[c.k] || 0) * 15);
          return `
            <div class="m-score-row" style="margin:3px 0">
              <span class="m-score-label" style="font-size:10px; min-width:65px">${c.l}</span>
              <div class="m-mini-bar"><div class="fill" style="width:${Math.min(100, (norm * 100)).toFixed(0)}%; opacity:0.75"></div></div>
            </div>
          `;
        }).join('')}

        <div class="m-props">
          ${props.map(p => `<div class="m-prop"><div class="p-v">${p.v}</div><div class="p-l">${p.l}</div></div>`).join('')}
        </div>

        ${compositionText}

        <div style="font-size:10px; color:var(--text-muted); margin-top:8px; line-height:1.5">${m.description || ''}</div>
      </div>
    `;
  },

  renderAlerts() {
    const alerts = this.state.alerts || [];
    const badge = document.getElementById('alertBadge');
    if (badge) {
      const count = alerts.filter(a => !a.resolved).length;
      badge.textContent = count;
      badge.style.display = count > 0 ? 'flex' : 'none';
    }

    const panel = document.getElementById('alertsPanel');
    if (!alerts.length) {
      panel.innerHTML = `<div class="empty-state"><div class="icon">✅</div>暂无活动告警<br><span style="font-size:11px">系统运行平稳</span></div>`;
      return;
    }

    panel.innerHTML = `
      <div class="panel" style="padding:10px">
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px">
          <span style="font-size:12px; color:var(--text-secondary)">共 ${alerts.length} 条活动告警</span>
          <span style="font-size:10px; color:var(--text-muted)">MQTT推送至文物保护中心</span>
        </div>
      </div>
      ${alerts.map(a => this.renderAlertItem(a)).join('')}
    `;
  },

  renderAlertItem(a) {
    const typeMap = {
      SETTLEMENT_EXCEEDED: '基础沉降超限',
      STRESS_EXCEEDED: '结构应力超限',
      WEATHERING_ACCELERATED: '风化速率加速',
      TILT_EXCEEDED: '结构倾角超限',
      LOAD_CAPACITY_LOW: '承载力严重不足',
      SENSOR_OFFLINE: '传感器离线',
      EQUIPMENT_FAULT: '设备故障'
    };
    const sevMap = { EMERGENCY: '紧急', CRITICAL: '严重', WARNING: '注意', INFO: '信息' };
    const typeLabel = typeMap[a.alert_type] || a.alert_type;

    return `
      <div class="alert-item" data-severity="${a.severity}">
        <div class="a-head">
          <div class="a-title">${a.title}</div>
          <span class="a-severity">${sevMap[a.severity] || a.severity}</span>
        </div>
        <div class="a-desc">${a.description || ''}</div>
        <div class="a-meta">
          <span>${a.aqueduct_name || ''} · ${typeLabel}</span>
          ${a.measured_value ? `<span>实测: <span class="val">${fmt.num(a.measured_value, 2)}${a.unit || ''}</span></span>` : ''}
          ${a.threshold_value ? `<span>阈值: ${fmt.num(a.threshold_value, 2)}${a.unit || ''}</span>` : ''}
          <span>${fmt.time(a.triggered_at)}</span>
        </div>
        <div style="margin-top:5px; font-size:9px; color:${a.mqtt_published ? 'var(--safe)' : 'var(--text-muted)'}">
          ${a.mqtt_published ? '✓ MQTT已推送 ' + (a.mqtt_message_id || '') : '⌛ MQTT待推送'}
          ${a.acknowledged ? ' · ✓ 已确认' : ''}
          ${a.resolved ? ' · ✅ 已处理' : ''}
        </div>
      </div>
    `;
  },

  switchTab(name) {
    this.state.currentTab = name;
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === name));
    document.querySelectorAll('.tab-content').forEach(t => t.classList.toggle('active', t.id === `tab-${name}`));
    try {
      const seg = this.state.currentSegment;
      const aq = this.state.currentAqueduct;
      if (name === 'inversion' && window.featureViz) {
        featureViz.renderInversionPanel('inversionPanel', seg);
      } else if (name === 'seismic' && window.featureViz) {
        featureViz.renderSeismicPanel('seismicPanel', aq, seg);
      } else if (name === 'lifetime' && window.featureViz) {
        featureViz.renderLifetimePanel('lifetimePanel', seg);
      } else if (name === 'tourism' && window.featureViz) {
        featureViz.renderTourismPanel('tourismPanel', this.state.aqueducts);
      }
    } catch (e) { console.warn(e); }
  },

  async refresh(silent) {
    if (!silent) this.toast('info', '正在刷新', '从后端获取最新数据...');
    try {
      this.state.stats = await api.getStats();
      this.renderHeaderStats();
      const newAlerts = await api.getAlerts(this.state.currentAqueduct?.id);
      if (JSON.stringify(newAlerts) !== JSON.stringify(this.state.alerts)) {
        this.state.alerts = newAlerts;
        this.renderAlerts();
      }
      if (this.state.currentAqueduct && this.state.currentSegment) {
        try {
          const detail = await api.getSegmentDetail(this.state.currentSegment.id);
          this.renderDetailPanel(detail);
        } catch (e) {}
      }
      if (!silent) this.toast('success', '刷新完成', new Date().toLocaleTimeString('zh-CN'));
    } catch (e) {
      if (!silent) this.toast('error', '刷新失败', e.message);
    }
  },

  async runFullEvaluation() {
    this.toast('info', '运行全量评估', '正在执行结构安全评估...');
    try {
      const r = await api.runFullEvaluation();
      this.toast('success', '评估完成', `评估 ${r.data?.segments_evaluated || 0} 段，生成 ${r.data?.alerts_generated || 0} 条告警`);
      this.refresh(true);
      if (this.state.currentAqueduct) this.selectAqueduct(this.state.currentAqueduct);
    } catch (e) {
      this.toast('error', '评估失败', e.message);
    }
  },

  toast(type, title, msg) {
    const container = document.getElementById('toastContainer');
    const t = document.createElement('div');
    t.className = `toast ${type}`;
    t.innerHTML = `<div class="t-title">${title}</div>${msg ? `<div class="t-msg">${msg}</div>` : ''}`;
    container.appendChild(t);
    setTimeout(() => {
      t.style.transition = 'opacity 0.4s, transform 0.4s';
      t.style.opacity = '0';
      t.style.transform = 'translateX(100%)';
      setTimeout(() => t.remove(), 500);
    }, 4000);
  },

  renderWeatheringLegend() {
  }
};

document.addEventListener('DOMContentLoaded', () => app.init());
