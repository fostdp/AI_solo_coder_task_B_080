const seismicFragility = {
  renderPanel(panelId, aqueduct, seg) {
    const p = document.getElementById(panelId);
    if (!p) return;
    p.innerHTML = `
      <h3>🌋 地震易损性评估</h3>
      <p class="panel-hint">增量动力分析 · 易损性曲线 · 区域风险地图</p>
      <div class="tab-group-mini">
        <button class="mini-tab active" id="smRisk" onclick="seismicFragility.switchTab('risk')">区域风险</button>
        <button class="mini-tab" id="smFrag" onclick="seismicFragility.switchTab('frag')">易损曲线</button>
        <button class="mini-tab" id="smQuake" onclick="seismicFragility.switchTab('quake')">历史地震</button>
      </div>
      <div id="seismicRiskView"></div>
      <div id="seismicFragView" style="display:none"></div>
      <div id="seismicQuakeView" style="display:none"></div>
    `;
    this.loadRisk(aqueduct);
    if (seg) this.loadFragility(seg);
    this.loadHistoricalQuakes();
  },

  switchTab(kind) {
    ['smRisk', 'smFrag', 'smQuake'].forEach(k => {
      const el = document.getElementById(k);
      if (el) el.classList.toggle('active', k === 'sm' + kind.charAt(0).toUpperCase() + kind.slice(1));
    });
    ['Risk', 'Frag', 'Quake'].forEach(k => {
      const el = document.getElementById('seismic' + k + 'View');
      if (el) el.style.display = (k.toLowerCase() === kind) ? 'block' : 'none';
    });
  },

  async loadRisk(aqueduct) {
    const v = document.getElementById('seismicRiskView');
    if (!v) return;
    v.innerHTML = `<div class="loading-spinner"></div>`;
    try {
      if (aqueduct) {
        const r = await api.analyzeSeismicRisk(aqueduct.id);
        const lvCls = r.overall_risk_level.includes('HIGH') ? 'crit' : (r.overall_risk_level.includes('MOD') ? 'warn' : 'good');
        v.innerHTML = `
          <h4>📍 ${aqueduct.name} · 风险等级</h4>
          <div class="risk-badge ${lvCls}">${r.overall_risk_level.replace('_', ' ')}</div>
          <div class="stat-row"><span>场地类别:</span><b>Class ${r.site_class}</b></div>
          <div class="stat-row"><span>土层放大系数:</span><b>${fmt.num(r.soil_amplification, 2)}×</b></div>
          <div class="stat-row"><span>卓越周期:</span><b>${fmt.num(r.predominant_period_sec, 3)} s</b></div>
          <div class="stat-row"><span>475年遇PGA:</span><b>${fmt.num(r.peak_ground_accel_475yr * 100, 2)} gal</b></div>
          <div class="stat-row"><span>2475年遇PGA:</span><b>${fmt.num(r.peak_ground_accel_2475yr * 100, 2)} gal</b></div>
          <div class="stat-row"><span>易损段数:</span><b class="${r.vulnerable_segments > 0 ? 'warn' : 'good'}">${r.vulnerable_segments} 段</b></div>
          <div class="stat-row"><span>期望经济损失:</span><b>€${fmt.num(r.estimated_total_loss, 0)}</b></div>
        `;
      }
      const all = await api.getAllSeismicRisks();
      const map = this.drawRiskMap(all || []);
      v.innerHTML += `<h4>🗺️ 区域风险地图</h4>${map}`;
    } catch (e) {
      v.innerHTML = `<div class="panel-hint bad">加载失败: ${e.message}</div>`;
    }
  },

  drawRiskMap(list) {
    const colors = { VERY_LOW: '#4CAF50', LOW: '#8BC34A', MODERATE: '#FFEB3B', HIGH: '#FF9800', VERY_HIGH: '#F44336' };
    const W = 340, H = 220;
    let dots = '';
    list.forEach(r => {
      const x = ((r.aqueduct_lng || 12.5) - 12.2) / 1.2 * W;
      const y = 1.0 - ((r.aqueduct_lat || 41.9) - 41.7) / 0.9;
      const c = colors[r.overall_risk_level] || '#888';
      const sz = 6 + Math.min(14, r.peak_ground_accel_475yr * 40);
      dots += `<circle cx="${x}" cy="${y * H}" r="${sz}" fill="${c}" opacity="0.82" stroke="#fff" stroke-width="1.5">
        <title>${r.aqueduct_name}: ${r.overall_risk_level} | PGA=${fmt.num(r.peak_ground_accel_475yr*100,1)}gal</title></circle>`;
    });
    let legend = '';
    Object.entries(colors).forEach(([k, c]) => {
      legend += `<div class="legend-item"><span class="dot" style="background:${c}"></span>${k.replace('_', ' ')}</div>`;
    });
    return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;background:linear-gradient(135deg,#1a2b3d,#253850);border-radius:6px">
      <defs><pattern id="grid" width="34" height="22" patternUnits="userSpaceOnUse"><path d="M 34 0 L 0 0 0 22" fill="none" stroke="#334a63" stroke-width="0.5"/></pattern></defs>
      <rect width="100%" height="100%" fill="url(#grid)"/>
      <path d="M 10 ${H-30} Q ${W/2} ${H/2}, ${W-20} 20" stroke="#5a7a9a" fill="none" stroke-width="2" stroke-dasharray="3,2"/>
      ${dots}
    </svg><div class="legend-safety">${legend}</div>`;
  },

  async loadFragility(seg) {
    const v = document.getElementById('seismicFragView');
    if (!v) return;
    if (!seg) {
      v.innerHTML = `<p class="panel-hint">请选择结构段以查看易损性曲线</p>`;
      return;
    }
    v.innerHTML = `<div class="loading-spinner"></div>`;
    try {
      const curve = await api.getFragilityCurve(seg.id);
      const states = ['Slight', 'Moderate', 'Extensive', 'Complete'];
      const clrs = { Slight: '#8BC34A', Moderate: '#FFEB3B', Extensive: '#FF9800', Complete: '#F44336' };
      const W = 340, H = 200, pad = 35;
      const maxX = 1.5;
      const toX = v => pad + (v / maxX) * (W - 2 * pad);
      const toY = v => (H - pad) - v * (H - 2 * pad);
      let paths = '';
      let labels = '';
      states.forEach(s => {
        let d = '';
        const key = s.toLowerCase() + '_prob';
        curve.forEach((pt, i) => {
          const val = pt[key] || 0;
          const x = toX(pt.pga_g), y = toY(val);
          d += (i === 0 ? 'M' : 'L') + x + ',' + y + ' ';
        });
        paths += `<path d="${d}" fill="none" stroke="${clrs[s]}" stroke-width="2.5"/>`;
        const lastP = curve[curve.length - 1];
        const endX = toX(lastP.pga_g);
        labels += `<text x="${Math.min(W - pad - 40, endX + 5)}" y="${toY(lastP[key] || 0) + 4}" fill="${clrs[s]}" font-size="10">${s}</text>`;
      });
      let axes = `<line x1="${pad}" y1="${H - pad}" x2="${W - pad}" y2="${H - pad}" stroke="#888"/>
        <line x1="${pad}" y1="${pad}" x2="${pad}" y2="${H - pad}" stroke="#888"/>`;
      for (let i = 0; i <= 5; i++) {
        axes += `<line x1="${pad + i * (W - 2 * pad) / 5}" y1="${H - pad}" x2="${pad + i * (W - 2 * pad) / 5}" y2="${H - pad + 4}" stroke="#888"/>
          <text x="${pad + i * (W - 2 * pad) / 5}" y="${H - pad + 16}" text-anchor="middle" fill="#aaa" font-size="9">${(i * maxX / 5).toFixed(2)}</text>`;
        axes += `<line x1="${pad - 4}" y1="${H - pad - i * (H - 2 * pad) / 5}" x2="${pad}" y2="${H - pad - i * (H - 2 * pad) / 5}" stroke="#888"/>
          <text x="${pad - 8}" y="${H - pad - i * (H - 2 * pad) / 5 + 3}" text-anchor="end" fill="#aaa" font-size="9">${(i * 0.2).toFixed(1)}</text>`;
      }
      v.innerHTML = `
        <h4>📈 易损性曲线</h4>
        <svg viewBox="0 0 ${W} ${H}" style="width:100%">${axes}${paths}${labels}
          <text x="${W / 2}" y="${H - 2}" text-anchor="middle" fill="#ccc" font-size="10">PGA (g)</text>
          <text x="8" y="${H / 2}" text-anchor="middle" fill="#ccc" font-size="10" transform="rotate(-90 8,${H / 2})">超越概率</text>
        </svg>`;
    } catch (e) {
      v.innerHTML = `<div class="panel-hint bad">加载失败: ${e.message}</div>`;
    }
  },

  async loadHistoricalQuakes() {
    const v = document.getElementById('seismicQuakeView');
    if (!v) return;
    v.innerHTML = `<div class="loading-spinner"></div>`;
    try {
      const list = await api.getHistoricalEarthquakes();
      let rows = '';
      list.slice(0, 10).forEach(q => {
        const mCls = q.magnitude >= 6.5 ? 'crit' : (q.magnitude >= 6 ? 'warn' : 'good');
        rows += `<tr>
          <td>${q.event_date || '-'}</td><td>${q.event_name}</td>
          <td class="${mCls}"><b>${fmt.num(q.magnitude, 1)}</b></td>
          <td>${q.region || '-'}</td>
          <td>${q.intensity_msk ? fmt.num(q.intensity_msk, 1) + '°' : '-'}</td>
        </tr><tr><td colspan="5" class="td-hint">${q.damage_description || ''}</td></tr>`;
      });
      v.innerHTML = `
        <h4>📜 历史地震记录</h4>
        <table class="data-table small">
          <thead><tr><th>日期</th><th>事件</th><th>震级</th><th>区域</th><th>烈度</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>`;
    } catch (e) {
      v.innerHTML = `<div class="panel-hint bad">加载失败: ${e.message}</div>`;
    }
  }
};
