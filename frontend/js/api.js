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
  async runFullEvaluation() { return this.post("/evaluation/run"); }
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
