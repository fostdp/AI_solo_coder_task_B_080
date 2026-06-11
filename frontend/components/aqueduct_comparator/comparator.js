const aqueductComparator = {
  renderPanel(panelId, aqueducts) {
    const p = document.getElementById(panelId);
    if (!p) return;
    p.innerHTML = `
      <h3>🎯 多水道对比与旅游规划</h3>
      <p class="panel-hint">雷达图对比 · 承载力评估 · 开放优先级排序</p>
      <div class="form-group"><label>选择要对比的水道 (Ctrl多选):</label></div>
      <div id="aqCheckboxes" style="max-height:140px;overflow-y:auto;border:1px solid #2d4258;padding:6px;border-radius:4px"></div>
      <div style="display:flex;gap:6px;margin:8px 0;flex-wrap:wrap">
        <button class="btn btn-primary" onclick="aqueductComparator.runComparison()">📊 执行对比</button>
        <button class="btn btn-refresh" onclick="aqueductComparator.selectAll(true)">全选</button>
        <button class="btn btn-refresh" onclick="aqueductComparator.selectAll(false)">清空</button>
      </div>
      <div id="tourismResult"></div>
    `;
    const cbWrap = document.getElementById('aqCheckboxes');
    (aqueducts || []).forEach(aq => {
      cbWrap.innerHTML += `<label style="display:block;padding:2px"><input type="checkbox" value="${aq.id}" class="aqcb" checked> ${aq.name}</label>`;
    });
    this.runComparison();
  },

  selectAll(on) {
    document.querySelectorAll('.aqcb').forEach(cb => cb.checked = on);
  },

  async runComparison() {
    const ids = [...document.querySelectorAll('.aqcb:checked')].map(c => c.value);
    const out = document.getElementById('tourismResult');
    if (!out) return;
    out.innerHTML = `<div class="loading-spinner"></div><p class="panel-hint">正在分析...</p>`;
    try {
      const r = await api.compareAqueducts(ids);
      const rd = r.radar_chart_data || {};
      const axes = rd.axes || [];
      const aqs = rd.aqueducts || {};
      const aqNames = Object.keys(aqs);
      const palette = ['#29b6f6', '#f06292', '#aed581', '#ffb74d', '#ba68c8', '#4db6ac', '#ffd54f', '#e57373'];
      const W = 340, H = 320, cx = W / 2, cy = H / 2, R = Math.min(W, H) / 2 - 40;
      const n = axes.length;
      let grid = '';
      for (let ring = 1; ring <= 5; ring++) {
        let pts = '';
        for (let i = 0; i < n; i++) {
          const a = (Math.PI * 2 * i / n) - Math.PI / 2;
          const rR = R * ring / 5;
          pts += (i === 0 ? 'M' : 'L') + (cx + Math.cos(a) * rR) + ',' + (cy + Math.sin(a) * rR) + ' ';
        }
        grid += `<path d="${pts}Z" fill="none" stroke="#3a5471" stroke-width="0.8"/>`;
        grid += `<text x="${cx + 2}" y="${cy - R * ring / 5 + 10}" fill="#888" font-size="9">${(ring * 20)}%</text>`;
      }
      let spoke = '', axisLabel = '';
      for (let i = 0; i < n; i++) {
        const a = (Math.PI * 2 * i / n) - Math.PI / 2;
        const x = cx + Math.cos(a) * R, y = cy + Math.sin(a) * R;
        spoke += `<line x1="${cx}" y1="${cy}" x2="${x}" y2="${y}" stroke="#3a5471"/>`;
        const lx = cx + Math.cos(a) * (R + 18);
        const ly = cy + Math.sin(a) * (R + 18) + 3;
        axisLabel += `<text x="${lx}" y="${ly}" text-anchor="middle" fill="#c9a849" font-size="10">${axes[i]}</text>`;
      }
      let plots = '', legend = '';
      aqNames.forEach((name, idx) => {
        const color = palette[idx % palette.length];
        let pts = '';
        const data = aqs[name] || [];
        for (let i = 0; i < n; i++) {
          const v = (data[i] && data[i].value) || 0;
          const a = (Math.PI * 2 * i / n) - Math.PI / 2;
          const rR = R * v;
          pts += (i === 0 ? 'M' : 'L') + (cx + Math.cos(a) * rR) + ',' + (cy + Math.sin(a) * rR) + ' ';
        }
        plots += `<path d="${pts}Z" fill="${color}" opacity="0.18" stroke="${color}" stroke-width="2"/>`;
        legend += `<div class="legend-item"><span class="dot" style="background:${color}"></span>${name}</div>`;
      });
      const rank = r.priority_ranking || {};
      const order = rank.priority_order || [];
      let rankRows = '';
      order.forEach(o => {
        const cls = o.category === 'RECOMMENDED' ? 'good' : (o.category === 'MONITOR_FIRST' ? 'crit' : 'warn');
        rankRows += `<tr>
          <td><b>#${o.rank}</b></td><td>${o.aqueduct_name}</td>
          <td>${fmt.pct(o.priority_score, 0)}</td>
          <td>${fmt.pct(o.safety_score, 0)}</td>
          <td class="${cls}">${o.category.replace(/_/g, ' ')}</td>
        </tr><tr><td colspan="5" class="td-hint">${o.description || ''}</td></tr>`;
      });
      const structM = r.structural_metrics || {};
      let compRows = '';
      Object.entries(structM).forEach(([name, m]) => {
        m = m || {};
        const sc = m.safety_score || 0;
        const scCls = sc >= 0.7 ? 'good' : (sc >= 0.5 ? 'warn' : 'crit');
        compRows += `<tr>
          <td>${name}</td>
          <td class="${scCls}"><b>${fmt.pct(sc, 0)}</b></td>
          <td>${fmt.pct(m.condition_score, 0)}</td>
          <td>€${fmt.num(m.repair_cost_estimate || 0, 0)}</td>
          <td>${fmt.num(m.visitor_per_year || 0, 0)}</td>
        </tr>`;
      });
      out.innerHTML = `
        <div class="result-card">
          <h4>🧭 雷达图对比</h4>
          <svg viewBox="0 0 ${W} ${H}" style="width:100%">${grid}${spoke}${plots}${axisLabel}</svg>
          <div class="legend-safety">${legend}</div>
          <h4>🏆 开放优先级排名</h4>
          <table class="data-table small">
            <thead><tr><th>排名</th><th>水道</th><th>综合分</th><th>安全分</th><th>类别</th></tr></thead>
            <tbody>${rankRows}</tbody>
          </table>
          <h4>📋 指标对比表</h4>
          <table class="data-table small">
            <thead><tr><th>水道</th><th>安全度</th><th>条件分</th><th>修复预算(€)</th><th>年游客量</th></tr></thead>
            <tbody>${compRows}</tbody>
          </table>
          <div class="recommendation-box">
            <b>💡 建议: </b>${r.recommendation_summary || ''}
          </div>
        </div>`;
    } catch (e) {
      out.innerHTML = `<div class="panel-hint bad">分析失败: ${e.message}</div>`;
    }
  }
};
