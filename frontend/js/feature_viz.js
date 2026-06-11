const featureViz = {
  renderInversionPanel(panelId, seg) {
    durabilityInverter.renderPanel(panelId, seg);
  },

  async runInversion(segId) {
    await durabilityInverter.runInversion(segId);
  },

  renderSeismicPanel(panelId, aqueduct, seg) {
    seismicFragility.renderPanel(panelId, aqueduct, seg);
  },

  seismicSwitch(kind) {
    seismicFragility.switchTab(kind);
  },

  async loadSeismicRisk(aqueduct) {
    await seismicFragility.loadRisk(aqueduct);
  },

  drawRiskMap(list) {
    return seismicFragility.drawRiskMap(list);
  },

  async loadSeismicFrag(seg) {
    await seismicFragility.loadFragility(seg);
  },

  async loadHistoricalQuakes() {
    await seismicFragility.loadHistoricalQuakes();
  },

  renderLifetimePanel(panelId, segment) {
    materialPredictor.renderPanel(panelId, segment);
  },

  async predictForSegment(seg) {
    await materialPredictor.predictForSegment(seg);
  },

  async runLifetime() {
    await materialPredictor.runPrediction();
  },

  renderTourismPanel(panelId, aqueducts) {
    aqueductComparator.renderPanel(panelId, aqueducts);
  },

  selectAllAq(on) {
    aqueductComparator.selectAll(on);
  },

  async runTourismCompare() {
    await aqueductComparator.runComparison();
  }
};
