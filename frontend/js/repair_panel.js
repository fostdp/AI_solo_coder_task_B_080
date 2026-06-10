/**
 * 修复方案推荐面板类
 * 封装修复推荐面板的所有UI逻辑，支持展示材料推荐、多属性决策、成本估算等信息
 * 
 * 依赖全局变量 api 和 fmt（由 api.js 提供）
 * 
 * 使用示例：
 * const panel = new RepairPanel('repairPanel', {
 *   onMaterialSelect: (material, index) => { ... }
 * });
 * panel.loadRecommendation('segment-123');
 */
export default class RepairPanel {
  /**
   * 构造函数
   * @param {string} containerId - 容器元素的ID
   * @param {Object} options - 配置选项
   * @param {Function} options.onMaterialSelect - 材料卡片点击回调 (material, index) => void
   * @param {boolean} options.showSensitivity - 是否显示灵敏度分析，默认 true
   * @param {boolean} options.showDecisionTable - 是否显示决策属性详情表格，默认 true
   */
  constructor(containerId, options = {}) {
    this.container = document.getElementById(containerId);
    this.options = {
      showSensitivity: true,
      showDecisionTable: true,
      ...options
    };

    this.currentSegmentId = null;
    this.currentSegmentName = null;
    this.currentRecommendation = null;
    this.isLoading = false;
    this.isVisible = true;

    if (this.container) {
      this.container.style.position = 'relative';
      this.container.style.display = 'flex';
      this.container.style.flexDirection = 'column';
      this.container.style.gap = '12px';
    }

    this._damageTypeMap = {
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

    this._materialTypeMap = {
      ROMAN_CONCRETE: '古罗马混凝土 · 文物友好',
      MODERN_CEMENT: '现代水泥基材料',
      EPOXY: '环氧树脂',
      GROUT: '注浆材料',
      FRP: '纤维增强聚合物',
      STONE_PATCH: '石材修补砂浆',
      LIME_MORTAR: '传统石灰砂浆'
    };

    this._decisionCriteria = [
      { key: 'compressive_strength', label: '抗压强度', defaultWeight: 0.15 },
      { key: 'tensile_strength', label: '抗拉强度', defaultWeight: 0.10 },
      { key: 'durability_rating', label: '耐久性', defaultWeight: 0.15 },
      { key: 'compatibility_rating', label: '相容性', defaultWeight: 0.15 },
      { key: 'aesthetic_match', label: '外观匹配', defaultWeight: 0.12 },
      { key: 'cost_per_unit', label: '成本(逆向)', defaultWeight: 0.12 },
      { key: 'construction_ease', label: '施工难度', defaultWeight: 0.08 },
      { key: 'environmental_impact', label: '环境影响', defaultWeight: 0.07 },
      { key: 'maintainability', label: '可维护性', defaultWeight: 0.06 }
    ];
  }

  /**
   * 加载并显示某段的修复推荐
   * @param {string} segmentId - 结构段ID
   * @param {string} [segmentName] - 结构段名称（可选）
   * @returns {Promise<Object>} 推荐数据
   */
  async loadRecommendation(segmentId, segmentName = null) {
    this.currentSegmentId = segmentId;
    this.currentSegmentName = segmentName;
    this.setLoading(true);
    this.setError(null);

    try {
      const recommendation = await api.getRepairRecommendation(segmentId);
      this.currentRecommendation = recommendation;
      this.render(recommendation);
      return recommendation;
    } catch (error) {
      console.error('加载修复推荐失败:', error);
      this.setError('加载修复推荐失败: ' + error.message);
      throw error;
    } finally {
      this.setLoading(false);
    }
  }

  /**
   * 设置当前段名称
   * @param {string} name - 段名称
   */
  setSegmentName(name) {
    this.currentSegmentName = name;
    if (this.currentRecommendation) {
      this.render(this.currentRecommendation);
    }
  }

  /**
   * 渲染推荐结果
   * @param {Object} recommendation - 推荐数据
   */
  render(recommendation) {
    if (!this.container) return;

    if (!recommendation) {
      this._renderEmpty();
      return;
    }

    const materials = recommendation.recommended_materials || [];
    const damageType = this._formatDamageType(recommendation.damage_type);
    const severity = recommendation.damage_severity || 0;
    const segmentName = this.currentSegmentName || recommendation.segment_name || '结构段';
    const context = (recommendation.decision_scores?.damage_analysis || {});

    this.container.innerHTML = `
      ${this._renderLoadingOverlay()}
      
      <div class="panel" style="background: linear-gradient(135deg, rgba(201, 168, 73, 0.08), var(--bg-panel));">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:8px">
          <div>
            <h3 style="margin:0; font-size:14px; color:var(--accent-light); font-weight:600">
              🔧 ${segmentName}
            </h3>
            <div style="font-size:11px; color:var(--text-muted); margin-top:2px">
              修复方案推荐
            </div>
          </div>
          <span class="badge" style="
            background: ${severity >= 0.6 ? 'var(--critical)' : severity >= 0.3 ? 'var(--warning)' : 'var(--safe)'};
            color: ${severity >= 0.3 ? '#1a1a1a' : '#1a1a1a'};
            padding: 4px 10px;
            border-radius: 10px;
            font-size: 10px;
            font-weight: 700;
          ">
            ${damageType}
          </span>
        </div>
        <div class="da-sev" style="display:flex; align-items:center; gap:8px; font-size:12px; color:var(--text-secondary)">
          <span>损伤严重度:</span>
          <div class="bar" style="flex:1; height:8px; background:var(--bg-primary); border-radius:4px; overflow:hidden">
            <div class="fill" style="height:100%; width:${(severity * 100).toFixed(0)}%; background:linear-gradient(90deg, #4CAF50, #FF9800, #F44336)"></div>
          </div>
          <span style="font-family:monospace; font-weight:700; color:var(--accent); min-width:42px">${(severity * 100).toFixed(0)}%</span>
        </div>
        <div style="margin-top:8px; font-size:11px; color:var(--text-muted)">
          紧急度：<b style="color:${severity >= 0.6 ? 'var(--critical)' : severity >= 0.3 ? 'var(--warning)' : 'var(--safe)'}">
            ${context.urgency_level === 'CRITICAL' ? '立即实施' : 
              context.urgency_level === 'URGENT' ? '尽快实施' : 
              context.urgency_level === 'SCHEDULED' ? '计划实施' : '预防性维护'}
          </b>
          ${context.heritage_compliance ? ' · 文物合规：✓' : ''}
          ${context.load_bearing_critical ? ' · 承重关键：✓' : ''}
        </div>
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">⭐ TOPSIS多属性决策推荐 · Top${Math.min(3, materials.length)}</div>
        <div style="font-size:10px; color:var(--text-muted); margin-bottom:8px">
          决策权重：相容性 ${this._getWeightPercent(recommendation, 'compatibility_rating')}% ·
          耐久性 ${this._getWeightPercent(recommendation, 'durability_rating')}% ·
          外观匹配 ${this._getWeightPercent(recommendation, 'aesthetic_match')}%
        </div>
        ${materials.slice(0, 3).map((m, idx) => this._renderMaterialCard(m, idx)).join('')}
      </div>

      ${this.options.showDecisionTable ? `
        <div class="panel">
          <div class="section-title" style="margin-top:0">📊 决策属性详情</div>
          <div style="font-size:10px; color:var(--text-muted); margin-bottom:8px">
            9个决策属性权重与得分对比（Top1材料）
          </div>
          ${this._renderDecisionTable(materials[0], recommendation)}
        </div>
      ` : ''}

      <div class="panel">
        <div class="section-title" style="margin-top:0">💰 修复造价估算</div>
        <div class="cost-summary">
          <div class="cs-row"><span class="label">材料费（综合加权）</span><span class="val">¥ ${fmt.num(recommendation.expected_cost * 0.55, 0)}</span></div>
          <div class="cs-row"><span class="label">施工费（含脚手架）</span><span class="val">¥ ${fmt.num(recommendation.expected_cost * 0.30, 0)}</span></div>
          <div class="cs-row"><span class="label">监测与验收费</span><span class="val">¥ ${fmt.num(recommendation.expected_cost * 0.15, 0)}</span></div>
          <div class="cs-total cs-row">
            <span class="label"><b>总估算造价</b>（含设计、管理）</span>
            <span class="val">¥ ${fmt.num(recommendation.expected_cost, 0)}</span>
          </div>
          <div class="cs-row" style="margin-top:6px">
            <span class="label">预期修复后寿命</span>
            <span class="val" style="font-size:15px; color:var(--safe)">${recommendation.expected_lifespan_years || 50} 年</span>
          </div>
        </div>
      </div>

      <div class="panel">
        <div class="section-title" style="margin-top:0">📋 施工建议</div>
        <div class="da-notes" style="font-size:11px; color:var(--text-secondary); line-height:1.7; white-space:pre-line">
${recommendation.construction_notes || '根据结构特点制定详细施工方案'}
        </div>
      </div>

      ${this.options.showSensitivity ? this._renderSensitivityAnalysis(recommendation) : ''}
    `;

    this._bindEvents();
  }

  /**
   * 显示面板
   */
  show() {
    if (this.container) {
      this.container.style.display = '';
      this.isVisible = true;
    }
  }

  /**
   * 隐藏面板
   */
  hide() {
    if (this.container) {
      this.container.style.display = 'none';
      this.isVisible = false;
    }
  }

  /**
   * 设置加载状态
   * @param {boolean} isLoading - 是否加载中
   */
  setLoading(isLoading) {
    this.isLoading = isLoading;
    const overlay = this.container?.querySelector('.repair-panel-loading');
    if (overlay) {
      overlay.style.display = isLoading ? 'flex' : 'none';
    }
  }

  /**
   * 设置错误状态
   * @param {string|null} message - 错误消息，null表示清除错误
   */
  setError(message) {
    if (!this.container) return;

    let errorEl = this.container.querySelector('.repair-panel-error');
    if (message) {
      if (!errorEl) {
        errorEl = document.createElement('div');
        errorEl.className = 'repair-panel-error';
        errorEl.style.cssText = `
          background: rgba(244, 67, 54, 0.1);
          border: 1px solid var(--critical);
          border-radius: 6px;
          padding: 12px;
          margin-bottom: 12px;
          color: var(--critical);
          font-size: 12px;
        `;
        this.container.insertBefore(errorEl, this.container.firstChild);
      }
      errorEl.textContent = '⚠️ ' + message;
    } else if (errorEl) {
      errorEl.remove();
    }
  }

  _renderEmpty() {
    this.container.innerHTML = `
      <div class="empty-state">
        <div class="icon">🔧</div>
        选择结构段后生成修复方案推荐
      </div>
    `;
  }

  _renderLoadingOverlay() {
    return `
      <div class="repair-panel-loading" style="
        position: absolute;
        top: 0; left: 0; right: 0; bottom: 0;
        background: rgba(10, 16, 24, 0.85);
        display: ${this.isLoading ? 'flex' : 'none'};
        flex-direction: column;
        align-items: center;
        justify-content: center;
        z-index: 10;
        gap: 14px;
        border-radius: 8px;
        backdrop-filter: blur(4px);
      ">
        <div style="
          width: 40px; height: 40px;
          border: 3px solid rgba(201, 168, 73, 0.2);
          border-top-color: var(--accent);
          border-radius: 50%;
          animation: spin 0.9s linear infinite;
        "></div>
        <div style="font-size: 13px; color: var(--text-secondary); letter-spacing: 1px;">
          正在分析修复方案...
        </div>
      </div>
    `;
  }

  _renderMaterialCard(material, index) {
    const isTop = index === 0;
    const typeLabel = this._materialTypeMap[material.material_type] || material.material_type;
    const score = material.decision_score || 0;
    const weightedScores = material.weighted_scores || {};

    const quickProps = [
      { value: material.compressive_strength?.toFixed(1), label: '抗压(MPa)' },
      { value: material.durability_rating?.toFixed(1), label: '耐久/10' },
      { value: material.compatibility_rating?.toFixed(1), label: '相容/10' },
      { value: material.aesthetic_match?.toFixed(1), label: '外观/10' },
      { value: material.cost_per_unit?.toFixed(0), label: '¥/' + (material.unit || 'm³') },
      { value: this._calculateTopsisPercent(weightedScores) + '%', label: 'TOPSIS' }
    ];

    let compositionText = '';
    if (material.composition && typeof material.composition === 'object') {
      const parts = Object.entries(material.composition).map(([k, v]) => `${k}:${v}`);
      if (parts.length) {
        compositionText = `<div class="m-composition">配方：${parts.join(' · ')}</div>`;
      }
    }

    const scenarioTags = this._getScenarioTags(material);
    const advantages = this._getAdvantages(material);

    const miniCriteria = [
      { key: 'compressive_strength', label: '抗压强度' },
      { key: 'durability_rating', label: '耐久性' },
      { key: 'compatibility_rating', label: '相容性' },
      { key: 'aesthetic_match', label: '外观匹配' },
      { key: 'cost_per_unit', label: '成本(逆向)' }
    ];

    return `
      <div class="material-card ${isTop ? 'top' : ''}" data-material-index="${index}" style="cursor:${this.options.onMaterialSelect ? 'pointer' : 'default'}">
        <div class="m-name">${index + 1}. ${material.name}</div>
        <div class="m-type">${typeLabel}</div>

        ${scenarioTags.length > 0 ? `
          <div style="display:flex; gap:4px; flex-wrap:wrap; margin-bottom:8px">
            ${scenarioTags.map(tag => `
              <span style="
                display:inline-block;
                padding:2px 6px;
                background: rgba(33, 150, 243, 0.15);
                color: var(--info);
                border-radius: 4px;
                font-size: 9px;
                font-weight: 500;
              ">${tag}</span>
            `).join('')}
          </div>
        ` : ''}

        <div class="m-score-row">
          <span class="m-score-label">综合得分</span>
          <div class="m-mini-bar">
            <div class="fill" style="width:${(score * 100).toFixed(0)}%"></div>
          </div>
          <span class="m-score-val">${(score * 100).toFixed(1)}</span>
        </div>

        ${miniCriteria.map(c => {
          const norm = c.key === 'cost_per_unit' 
            ? (1 - Math.min(1, (weightedScores[c.key] || 0) * 3)) 
            : Math.min(1, (weightedScores[c.key] || 0) * 15);
          return `
            <div class="m-score-row" style="margin:3px 0">
              <span class="m-score-label" style="font-size:10px; min-width:65px">${c.label}</span>
              <div class="m-mini-bar"><div class="fill" style="width:${Math.min(100, (norm * 100)).toFixed(0)}%; opacity:0.75"></div></div>
            </div>
          `;
        }).join('')}

        <div class="m-props">
          ${quickProps.map(p => `
            <div class="m-prop">
              <div class="p-v">${p.value || '-'}</div>
              <div class="p-l">${p.label}</div>
            </div>
          `).join('')}
        </div>

        ${compositionText}

        ${advantages.length > 0 ? `
          <div style="margin-top:8px; font-size:10px; color:var(--text-muted)">
            <div style="margin-bottom:4px; color:var(--text-secondary); font-weight:500">主要优点：</div>
            <ul style="margin:0; padding-left:16px; line-height:1.5">
              ${advantages.map(adv => `<li>${adv}</li>`).join('')}
            </ul>
          </div>
        ` : ''}

        <div style="font-size:10px; color:var(--text-muted); margin-top:8px; line-height:1.5">
          ${material.description || ''}
        </div>
      </div>
    `;
  }

  _renderDecisionTable(topMaterial, recommendation) {
    if (!topMaterial) {
      return '<div style="color:var(--text-muted); font-size:11px">暂无数据</div>';
    }

    const weights = recommendation.decision_scores?.scenario_weights || [];
    const weightedScores = topMaterial.weighted_scores || {};

    const getWeight = (key) => {
      const found = weights.find(w => w.name === key);
      if (found) return found.weight;
      const criteria = this._decisionCriteria.find(c => c.key === key);
      return criteria ? criteria.defaultWeight : 0;
    };

    const getScore = (key) => {
      return weightedScores[key] !== undefined ? weightedScores[key] : (topMaterial[key] || 0);
    };

    const rows = this._decisionCriteria.map(criteria => {
      const weight = getWeight(criteria.key);
      const rawScore = getScore(criteria.key);
      const normalizedScore = criteria.key === 'cost_per_unit' 
        ? Math.max(0, 1 - Math.min(1, rawScore * 3))
        : Math.min(1, rawScore * 15);
      const finalScore = weight * normalizedScore;

      return {
        label: criteria.label,
        weight: weight,
        normalizedScore: normalizedScore,
        finalScore: finalScore
      };
    });

    const totalScore = rows.reduce((sum, r) => sum + r.finalScore, 0);

    return `
      <div style="overflow-x:auto;">
        <table style="width:100%; font-size:11px; border-collapse:collapse;">
          <thead>
            <tr style="border-bottom:1px solid var(--border);">
              <th style="text-align:left; padding:6px 4px; color:var(--text-muted); font-weight:500;">属性</th>
              <th style="text-align:right; padding:6px 4px; color:var(--text-muted); font-weight:500;">权重</th>
              <th style="text-align:right; padding:6px 4px; color:var(--text-muted); font-weight:500;">得分</th>
              <th style="text-align:right; padding:6px 4px; color:var(--text-muted); font-weight:500;">加权分</th>
            </tr>
          </thead>
          <tbody>
            ${rows.map(row => `
              <tr style="border-bottom:1px solid rgba(45, 68, 88, 0.5);">
                <td style="padding:6px 4px; color:var(--text-secondary);">${row.label}</td>
                <td style="text-align:right; padding:6px 4px; color:var(--accent); font-family:monospace;">${(row.weight * 100).toFixed(0)}%</td>
                <td style="text-align:right; padding:6px 4px;">
                  <div style="display:flex; align-items:center; justify-content:flex-end; gap:6px;">
                    <div style="width:60px; height:6px; background:var(--bg-primary); border-radius:3px; overflow:hidden;">
                      <div style="height:100%; width:${(row.normalizedScore * 100).toFixed(0)}%; background:linear-gradient(90deg, var(--accent-dark), var(--accent-light)); border-radius:3px;"></div>
                    </div>
                    <span style="color:var(--text-primary); font-family:monospace; width:36px; text-align:right;">${(row.normalizedScore * 100).toFixed(0)}%</span>
                  </div>
                </td>
                <td style="text-align:right; padding:6px 4px; color:var(--text-primary); font-family:monospace; font-weight:600;">${(row.finalScore * 100).toFixed(1)}</td>
              </tr>
            `).join('')}
            <tr style="background:rgba(201, 168, 73, 0.1);">
              <td style="padding:8px 4px; font-weight:600; color:var(--accent-light);">合计</td>
              <td style="text-align:right; padding:8px 4px; color:var(--accent); font-weight:600; font-family:monospace;">100%</td>
              <td style="padding:8px 4px;"></td>
              <td style="text-align:right; padding:8px 4px; color:var(--accent-light); font-weight:700; font-family:monospace; font-size:13px;">${(totalScore * 100).toFixed(1)}</td>
            </tr>
          </tbody>
        </table>
      </div>
    `;
  }

  _renderSensitivityAnalysis(recommendation) {
    const sensitivity = recommendation.sensitivity_analysis || {};
    const confidenceScore = sensitivity.confidence_score || 0;
    const criticalDecisions = sensitivity.critical_decisions || [];
    const attributeSensitivities = sensitivity.attribute_sensitivities || [];

    if (confidenceScore === 0 && criticalDecisions.length === 0 && attributeSensitivities.length === 0) {
      return '';
    }

    return `
      <div class="panel">
        <div class="section-title" style="margin-top:0">📐 灵敏度分析</div>
        
        <div style="margin-bottom:12px;">
          <div style="font-size:10px; color:var(--text-muted); margin-bottom:4px;">决策置信度</div>
          <div class="progress-bar">
            <div class="fill safe" style="width:${(confidenceScore * 100).toFixed(0)}%"></div>
            <div class="label">${(confidenceScore * 100).toFixed(1)}%</div>
          </div>
        </div>

        ${criticalDecisions.length > 0 ? `
          <div style="margin-bottom:12px;">
            <div style="font-size:11px; color:var(--text-secondary); margin-bottom:6px; font-weight:500;">
              ⚠️ 关键决策因素
            </div>
            <div style="display:flex; flex-direction:column; gap:4px;">
              ${criticalDecisions.map(d => `
                <div style="
                  font-size:10px;
                  padding:6px 8px;
                  background: rgba(255, 152, 0, 0.1);
                  border: 1px solid rgba(255, 152, 0, 0.3);
                  border-radius: 4px;
                  color: var(--warning);
                ">
                  ${d.name || d}${d.impact ? ` · 影响度: ${(d.impact * 100).toFixed(0)}%` : ''}
                </div>
              `).join('')}
            </div>
          </div>
        ` : ''}

        ${attributeSensitivities.length > 0 ? `
          <div>
            <div style="font-size:11px; color:var(--text-secondary); margin-bottom:6px; font-weight:500;">
              各属性灵敏度
            </div>
            <div style="display:flex; flex-direction:column; gap:3px;">
              ${attributeSensitivities.slice(0, 6).map(attr => `
                <div style="display:flex; align-items:center; gap:8px;">
                  <span style="font-size:10px; color:var(--text-muted); min-width:70px;">${attr.name || attr.attribute || ''}</span>
                  <div style="flex:1; height:6px; background:var(--bg-primary); border-radius:3px; overflow:hidden;">
                    <div style="
                      height:100%;
                      width:${((attr.sensitivity || attr.value || 0) * 100).toFixed(0)}%;
                      background:linear-gradient(90deg, var(--info), var(--accent));
                      border-radius:3px;
                    "></div>
                  </div>
                  <span style="font-size:10px; color:var(--text-primary); font-family:monospace; min-width:36px; text-align:right;">
                    ${((attr.sensitivity || attr.value || 0) * 100).toFixed(1)}%
                  </span>
                </div>
              `).join('')}
            </div>
          </div>
        ` : ''}
      </div>
    `;
  }

  _formatDamageType(damageType) {
    if (!damageType) return '未知损伤类型';
    return damageType.split('+').map(t => {
      return this._damageTypeMap[t.trim()] || t.trim();
    }).join(' + ');
  }

  _getWeightPercent(recommendation, weightName) {
    const weights = recommendation?.decision_scores?.scenario_weights || [];
    const found = weights.find(w => w.name === weightName);
    return found ? (found.weight * 100).toFixed(0) : '0';
  }

  _calculateTopsisPercent(weightedScores) {
    if (!weightedScores) return 0;
    const distanceWorst = weightedScores.distance_worst || 1;
    const distanceBest = weightedScores.distance_best || 1;
    const total = distanceWorst + distanceBest;
    if (total === 0) return 0;
    return Math.round((distanceWorst / total) * 100);
  }

  _getScenarioTags(material) {
    const tags = [];
    if (material.compatibility_rating >= 8) tags.push('文物兼容');
    if (material.durability_rating >= 8) tags.push('高耐久');
    if (material.aesthetic_match >= 8) tags.push('外观匹配');
    if (material.cost_per_unit && material.cost_per_unit < 2000) tags.push('经济实惠');
    if (material.material_type === 'ROMAN_CONCRETE' || material.material_type === 'LIME_MORTAR') {
      tags.push('传统工艺');
    }
    if (material.material_type === 'FRP' || material.material_type === 'EPOXY') {
      tags.push('高强度');
    }
    return tags.slice(0, 3);
  }

  _getAdvantages(material) {
    const advantages = [];
    if (material.compatibility_rating >= 9) {
      advantages.push('与原结构相容性极佳，文物保护友好');
    } else if (material.compatibility_rating >= 7) {
      advantages.push('与原结构相容性良好');
    }
    if (material.durability_rating >= 9) {
      advantages.push('耐久性优异，使用寿命长');
    } else if (material.durability_rating >= 7) {
      advantages.push('耐久性良好');
    }
    if (material.compressive_strength && material.compressive_strength >= 50) {
      advantages.push('抗压强度高，适用于承重结构');
    }
    if (material.aesthetic_match >= 8) {
      advantages.push('外观与原结构匹配度高');
    }
    if (material.cost_per_unit && material.cost_per_unit < 1500) {
      advantages.push('成本较低，经济性好');
    }
    if (material.construction_ease && material.construction_ease >= 8) {
      advantages.push('施工简便，工期短');
    }
    return advantages.slice(0, 3);
  }

  _bindEvents() {
    if (!this.options.onMaterialSelect) return;

    const cards = this.container.querySelectorAll('.material-card');
    cards.forEach((card, index) => {
      card.addEventListener('click', (e) => {
        e.stopPropagation();
        if (this.currentRecommendation?.recommended_materials) {
          this.options.onMaterialSelect(
            this.currentRecommendation.recommended_materials[index],
            index
          );
        }
      });
    });
  }
}
