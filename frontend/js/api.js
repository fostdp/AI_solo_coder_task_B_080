const API_BASE = "http://localhost:8080/api";

const api = {
  async get(endpoint) {
    const res = await fetch(API_BASE + endpoint, {
      headers: { "Accept": "application/json" }
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${await res.text()}`);
    return res.json();
  },

  async post(endpoint, body) {
    const res = await fetch(API_BASE + endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}: ${await res.text()}`);
    return res.json();
  },

  async health() { return this.get("/health"); },
  async getAqueducts() { return (await this.get("/aqueducts")).data; },
  async getAqueduct(id) { return (await this.get(`/aqueducts/${id}`)).data; },
  async getSegments(aqueductId) {
    const qs = aqueductId ? `?aqueduct_id=${aqueductId}` : "";
    return (await this.get(`/segments${qs}`)).data;
  },
  async getSegmentDetail(id) { return (await this.get(`/segments/${id}`)).data; },
  async getRepairRecommendation(segmentId) {
    return (await this.get(`/segments/${segmentId}/repair`)).data;
  },
  async getAlerts(aqueductId) {
    const qs = aqueductId ? `?aqueduct_id=${aqueductId}` : "";
    return (await this.get(`/alerts${qs}`)).data;
  },
  async getStats() { return (await this.get("/stats")).data; },
  async getMaterials() { return (await this.get("/materials")).data; },
  async runFullEvaluation() { return this.post("/evaluation/run"); },

  // Feature 1: 古罗马混凝土耐久性反演
  async invertConcrete(payload) { return (await this.post("/inversion/invert", payload)).data; },
  async getConcreteFormulas() { return (await this.get("/inversion/formulas")).data; },
  async getAqueductInversions(aqueductId) { return (await this.get(`/inversion/aqueducts/${aqueductId}`)).data; },

  // Feature 2: 地震易损性评估
  async getHistoricalEarthquakes() { return (await this.get("/seismic/earthquakes")).data; },
  async analyzeSeismicRisk(aqueductId) { return (await this.get(`/seismic/risks/${aqueductId}`)).data; },
  async getAllSeismicRisks() { return (await this.get("/seismic/risks")).data; },
  async getFragilityCurve(segmentId) { return (await this.get(`/seismic/fragility/${segmentId}`)).data; },
  async analyzeIDA(segmentId) { return (await this.get(`/seismic/ida/${segmentId}`)).data; },

  // Feature 3: 修复材料长期性能预测
  async predictMaterialLifetime(payload) { return (await this.post("/lifetime/predict", payload)).data; },
  async getMaterialPredictions(materialId) { return (await this.get(`/lifetime/materials/${materialId}`)).data; },

  // Feature 4: 多水道对比与旅游规划
  async compareAqueducts(aqueductIds) {
    return (await this.post("/tourism/compare", { aqueduct_ids: aqueductIds || [], save_result: false })).data;
  },
  async getRecentComparisons(limit) {
    const qs = limit ? `?limit=${limit}` : "";
    return (await this.get(`/tourism/comparisons${qs}`)).data;
  }
};

const fmt = {
  num(v, d = 2) {
    if (v === null || v === undefined || isNaN(v)) return "-";
    return Number(v).toLocaleString("zh-CN", { maximumFractionDigits: d, minimumFractionDigits: d });
  },
  pct(v, d = 1) {
    if (v === null || v === undefined || isNaN(v)) return "-";
    return (v * 100).toFixed(d) + "%";
  },
  capacityColor(v) {
    if (v >= 0.8) return "good";
    if (v >= 0.65) return "warn";
    if (v >= 0.5) return "bad";
    return "crit";
  },
  capacityFillClass(v) {
    if (v >= 0.8) return "safe";
    if (v >= 0.65) return "warn";
    if (v >= 0.5) return "danger";
    return "crit";
  },
  time(t) {
    if (!t) return "-";
    const d = new Date(t);
    return d.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
  }
};
