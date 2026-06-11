const durabilityInverter = {
  renderPanel(panelId, seg) {
    const p = document.getElementById(panelId);
    if (!p) return;
    if (!seg) {
      p.innerHTML = `<div class="panel-hint">请先在3D视图中选择一个结构段</div>`;
      return;
    }
    p.innerHTML = `
      <h3>🏺 古罗马混凝土耐久性反演</h3>
      <p class="panel-hint">基于现存砂浆风化深度与强度，反推原始配比与耐久性机制</p>
      <div class="form-group">
        <label>观测风化深度 (mm): <span id="invDepthVal">${fmt.num(seg.weathering_depth || 15, 1)}</span></label>
        <input type="range" id="invDepth" min="0" max="60" step="0.5" value="${seg.weathering_depth || 15}"
          oninput="document.getElementById('invDepthVal').innerText=this.value">
      </div>
      <div class="form-group">
        <label>观测剩余强度 (MPa): <span id="invStrVal">${fmt.num(seg.current_stress ? Math.max(2, 8 - seg.current_stress*0.5) : 6.5, 2)}</span></label>
        <input type="range" id="invStr" min="1" max="15" step="0.1" value="${seg.current_stress ? Math.max(2, 8 - seg.current_stress*0.5) : 6.5}"
          oninput="document.getElementById('invStrVal').innerText=this.value">
      </div>
      <div class="form-group">
        <label>结构龄期 (年): <span id="invAgeVal">2000</span></label>
        <input type="range" id="invAge" min="500" max="2500" step="50" value="2000"
          oninput="document.getElementById('invAgeVal').innerText=this.value">
      </div>
      <div class="form-group">
        <label>砂浆pH值: <span id="invPhVal">9.5</span></label>
        <input type="range" id="invPh" min="7.5" max="13" step="0.1" value="9.5"
          oninput="document.getElementById('invPhVal').innerText=this.value">
      </div>
      <button class="btn btn-primary" onclick="durabilityInverter.runInversion('${seg.id}')">🔬 执行反演分析</button>
      <div id="invResult"></div>
    `;
  },

  async runInversion(segId) {
    const depth = parseFloat(document.getElementById('invDepth').value);
    const str = parseFloat(document.getElementById('invStr').value);
    const age = parseFloat(document.getElementById('invAge').value);
    const ph = parseFloat(document.getElementById('invPh').value);
    const out = document.getElementById('invResult');
    out.innerHTML = `<div class="loading-spinner"></div><p class="panel-hint">正在反演求解...</p>`;
    try {
      const r = await api.invertConcrete({
        segment_id: segId,
        observed_weathering_mm: depth,
        observed_strength_mpa: str,
        age_years: age,
        observed_ph: ph,
        save_result: true
      });
      const confColor = r.inversion_confidence >= 0.7 ? 'good' : (r.inversion_confidence >= 0.5 ? 'warn' : 'bad');
      let candHtml = '';
      const cands = r.candidate_formulas && r.candidate_formulas.candidates ? r.candidate_formulas.candidates : [];
      cands.slice(0, 5).forEach((c, i) => {
        candHtml += `
          <tr>
            <td>${i + 1}</td>
            <td>${c.formula_name || '配方-' + i}</td>
            <td class="${i === 0 ? 'good' : ''}">${fmt.pct(c.match_score, 1)}</td>
            <td>${fmt.num(c.simulated_depth, 2)} mm</td>
            <td>${fmt.num(c.residual_error, 3)}</td>
          </tr>`;
      });
      const mech = r.inferred_durability_mechanism || {};
      const mod = r.modern_reference_formula || {};
      const modCmp = (mod.modern_opc_comparison) || {};
      out.innerHTML = `
        <div class="result-card">
          <div class="stat-row"><span>反演置信度:</span><b class="${confColor}">${fmt.pct(r.inversion_confidence)}</b></div>
          ${r.best_match_formula ? `
            <h4>🎯 最佳匹配: ${r.best_match_formula.formula_name}</h4>
            <div class="formula-ratios">
              <div class="ratio-item"><span>石灰</span><b>${fmt.num(r.best_match_formula.lime_ratio, 2)}</b></div>
              <div class="ratio-item"><span>火山灰</span><b>${fmt.num(r.best_match_formula.pozzolana_ratio, 2)}</b></div>
              <div class="ratio-item"><span>骨料</span><b>${fmt.num(r.best_match_formula.aggregate_ratio, 2)}</b></div>
              <div class="ratio-item"><span>水</span><b>${fmt.num(r.best_match_formula.water_ratio, 2)}</b></div>
            </div>
            <div class="stat-row"><span>推定原始强度:</span><b>${fmt.num(r.best_match_formula.original_fy_mpa, 2)} MPa</b></div>
            <div class="stat-row"><span>耐久性指数:</span><b>${fmt.num(r.best_match_formula.durability_index, 3)}</b></div>
          ` : `<h4>🎯 推定原始强度: ${fmt.num(r.inferred_original_fy, 2)} MPa</h4>`}
          <h4>📊 耐久机制解析</h4>
          <div class="stat-row"><span>钙溶出速率:</span><b>${fmt.num(r.leaching_rate * 1000, 3)} ×10⁻³ /年</b></div>
          <div class="stat-row"><span>碳化深度:</span><b>${fmt.num(r.carbonation_depth, 2)} mm</b></div>
          <div class="stat-row"><span>火山灰反应龄期:</span><b>${fmt.num(mech.pozzolanic_reaction_age_years || 0, 0)} 年</b></div>
          <div class="stat-row"><span>自修复潜力:</span><b class="${(mech.self_healing_potential || 0) >= 0.6 ? 'good' : 'warn'}">${fmt.pct(mech.self_healing_potential || 0)}</b></div>
          <h4>📋 候选配方排名</h4>
          <table class="data-table">
            <thead><tr><th>#</th><th>配方名</th><th>匹配度</th><th>模拟风化</th><th>残差</th></tr></thead>
            <tbody>${candHtml}</tbody>
          </table>
          <h4>🌐 现代混凝土参考</h4>
          <div class="stat-row"><span>与OPC耐久性比:</span><b>${fmt.num(modCmp.durability_ratio_roman_opc || 0, 2)}×</b></div>
          <div class="stat-row"><span>碳足迹 (OPC):</span><b>${fmt.pct(modCmp.carbon_footprint_pct_opc || 0)}</b></div>
          <p class="panel-hint"><b>工程建议:</b> ${mod.recommendation || '可参考火山灰掺量调整现代配比'}</p>
        </div>`;
    } catch (e) {
      out.innerHTML = `<div class="panel-hint bad">反演失败: ${e.message}</div>`;
    }
  }
};
