const materialPredictor = {
  renderPanel(panelId, segment) {
    const p = document.getElementById(panelId);
    if (!p) return;
    p.innerHTML = `
      <h3>⏳ 修复材料长期性能预测</h3>
      <p class="panel-hint">Arrhenius方程 + 时间-温度叠加原理，预测50/100年退化</p>
      <div class="form-group">
        <label>材料选择:</label>
        <select id="lifeMaterial"></select>
      </div>
      <div class="form-group">
        <label>环境场景:</label>
        <select id="lifeScenario">
          <option value="mediterranean">地中海气候 (罗马)</option>
          <option value="temperate_coastal">温带沿海</option>
          <option value="continental">大陆性气候</option>
          <option value="alpine">高山严寒</option>
          <option value="tropical_humid">热带潮湿</option>
          <option value="urban_polluted">城市污染</option>
          <option value="underwater_saline">水下盐水</option>
          <option value="laboratory_control">实验室对照</option>
        </select>
      </div>
      <button class="btn btn-primary" onclick="materialPredictor.runPrediction()">📉 预测退化曲线</button>
      <div id="lifeResult"></div>
    `;
    api.getMaterials().then(ms => {
      const sel = document.getElementById('lifeMaterial');
      if (!sel) return;
      (ms || []).forEach(m => {
        sel.innerHTML += `<option value="${m.id}">${m.name} (${m.material_type})</option>`;
      });
      if (!segment) return;
      this.predictForSegment(segment);
    });
  },

  async predictForSegment(seg) {
    try {
      const ms = await api.getMaterials();
      const sel = document.getElementById('lifeMaterial');
      if (!sel || !ms || ms.length === 0) return;
      sel.value = ms[0].id;
    } catch (e) {}
  },

  async runPrediction() {
    const mid = document.getElementById('lifeMaterial').value;
    const sc = document.getElementById('lifeScenario').value;
    const out = document.getElementById('lifeResult');
    out.innerHTML = `<div class="loading-spinner"></div><p class="panel-hint">Arrhenius模型拟合中...</p>`;
    try {
      const r = await api.predictMaterialLifetime({ material_id: mid, scenario: sc, save_result: true });
      const pts = (r.degradation_curve && r.degradation_curve.points) ? r.degradation_curve.points : [];
      const W = 340, H = 200, pad = 35;
      const maxX = r.prediction_years || 100;
      const toX = v => pad + (v / maxX) * (W - 2 * pad);
      const toY = v => (H - pad) - v * (H - 2 * pad);
      let line1 = '', line2 = '', line3 = '';
      pts.forEach((pt, i) => {
        const x = toX(pt.year), y = toY(pt.strength_ratio);
        const yl = toY(Math.max(0, pt.confidence_low || 0));
        const yh = toY(Math.min(1, pt.confidence_high || 1));
        line1 += (i === 0 ? 'M' : 'L') + x + ',' + y + ' ';
        line2 += (i === 0 ? 'M' : 'L') + x + ',' + yl + ' ';
        line3 += (i === 0 ? 'M' : 'L') + x + ',' + yh + ' ';
      });
      const fillPts = [];
      pts.forEach(pt => fillPts.push([pt.year, pt.confidence_low || 0, pt.confidence_high || 1));
      let area = '';
      fillPts.forEach((p, i) => {
        area += (i === 0 ? 'M' : 'L') + toX(p[0]) + ',' + toY(p[2]) + ' ';
      });
      for (let i = fillPts.length - 1; i >= 0; i--) {
        area += 'L' + toX(fillPts[i][0]) + ',' + toY(fillPts[i][1]) + ' ';
      }
      let axes = `<line x1="${pad}" y1="${H - pad}" x2="${W - pad}" y2="${H - pad}" stroke="#888"/>
        <line x1="${pad}" y1="${pad}" x2="${pad}" y2="${H - pad}" stroke="#888"/>`;
      for (let i = 0; i <= 5; i++) {
        axes += `<line x1="${pad + i * (W - 2 * pad) / 5}" y1="${H - pad}" x2="${pad + i * (W - 2 * pad) / 5}" y2="${H - pad + 4}" stroke="#888"/>
          <text x="${pad + i * (W - 2 * pad) / 5}" y="${H - pad + 16}" text-anchor="middle" fill="#aaa" font-size="9">${i * maxX / 5}a</text>`;
        axes += `<line x1="${pad - 4}" y1="${H - pad - i * (H - 2 * pad) / 5}" x2="${pad}" y2="${H - pad - i * (H - 2 * pad) / 5}" stroke="#888"/>
          <text x="${pad - 8}" y="${H - pad - i * (H - 2 * pad) / 5 + 3}" text-anchor="end" fill="#aaa" font-size="9">${(i * 0.2).toFixed(1)}</text>`;
      }
      const thr = toY(r.threshold_strength_ratio || 0.5);
      axes += `<line x1="${pad}" y1="${thr}" x2="${W - pad}" y2="${thr}" stroke="#c9a849" stroke-dasharray="3,2"/>
        <text x="${W - pad - 4}" y="${thr - 4}" text-anchor="end" fill="#c9a849" font-size="9">阈值 ${fmt.pct(r.threshold_strength_ratio)}</text>`;
      const f50 = toX(50), f100 = toX(100);
      const y50 = toY(r.strength_at_50yr), y100 = toY(r.strength_at_100yr);
      axes += `<circle cx="${f50}" cy="${y50}" r="4" fill="#4fc3f7"/><circle cx="${f100}" cy="${y100}" r="4" fill="#ff7043"/>`;
      const tts = (r.time_temp_shift_factor) || {};
      out.innerHTML = `
        <div class="result-card">
          <div class="stat-row"><span>预测材料:</span><b>${r.material_name || '-'}</b> (${r.material_type})</div>
          <div class="stat-row"><span>场景:</span><b>${sc.replace(/_/g, ' ')}</b></div>
          <div class="stat-row"><span>50年保留强度:</span><b class="${r.strength_at_50yr >= 0.7 ? 'good' : (r.strength_at_50yr >= 0.5 ? 'warn' : 'crit')}">${fmt.pct(r.strength_at_50yr)}</b></div>
          <div class="stat-row"><span>100年保留强度:</span><b class="${r.strength_at_100yr >= 0.5 ? 'good' : (r.strength_at_100yr >= 0.3 ? 'warn' : 'crit')}">${fmt.pct(r.strength_at_100yr)}</b></div>
          <div class="stat-row"><span>预测服役寿命:</span><b class="good">${fmt.num(r.estimated_service_life, 0)} 年</b></div>
          <div class="stat-row"><span>修复方案有效期:</span><b>约 ${r.repair_validity_years || 0} 年</b></div>
          <h4>📉 退化曲线</h4>
          <svg viewBox="0 0 ${W} ${H}" style="width:100%">${axes}
            <path d="${area}Z" fill="#4fc3f7" opacity="0.15"/>
            <path d="${line2}" fill="none" stroke="#4fc3f7" stroke-dasharray="2,2" stroke-width="1" opacity="0.7"/>
            <path d="${line3}" fill="none" stroke="#4fc3f7" stroke-dasharray="2,2" stroke-width="1" opacity="0.7"/>
            <path d="${line1}" fill="none" stroke="#29b6f6" stroke-width="2.5"/>
            <text x="${W / 2}" y="${H - 2}" text-anchor="middle" fill="#ccc" font-size="10">时间 (年)</text>
            <text x="8" y="${H / 2}" text-anchor="middle" fill="#ccc" font-size="10" transform="rotate(-90 8,${H / 2})">强度保留比</text>
          </svg>
          <p class="legend-mini"><span style="color:#4fc3f7">■</span> 预测值 <span style="opacity:0.6">░░</span>95%置信区间 <span style="color:#4fc3f7">●</span>50年点 <span style="color:#ff7043">●</span>100年点</p>
          <h4>🔬 模型参数</h4>
          <div class="stat-row"><span>活化能 Ea:</span><b>${fmt.num(r.arrhenius_activation_ev, 4)} eV</b></div>
          <div class="stat-row"><span>80°C加速因子:</span><b>${fmt.num(tts.time_accel_factor_80C || 0, 1)}×</b></div>
          <p class="panel-hint">${r.model_assumptions || ''}</p>
        </div>`;
    } catch (e) {
      out.innerHTML = `<div class="panel-hint bad">预测失败: ${e.message}</div>`;
    }
  }
};
