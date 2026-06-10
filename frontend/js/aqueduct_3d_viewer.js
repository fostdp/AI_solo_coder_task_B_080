import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { LOD } from 'three';

class Aqueduct3DViewer {
  constructor() {
    this.scene = null;
    this.camera = null;
    this.renderer = null;
    this.controls = null;
    this.raycaster = new THREE.Raycaster();
    this.mouse = new THREE.Vector2();
    this.segments = new Map();
    this.segmentMeshes = [];
    this.lodObjects = [];
    this.selectedId = null;
    this.hoveredId = null;
    this.colorMode = 'weathering';
    this.wireframe = false;
    this.canvas = null;
    this.minimapCanvas = null;
    this.tooltipEl = null;
    this.hudEl = null;
    this.onClickCallback = null;
    this.onSegmentClick = null;

    this.isMobile = false;
    this.isLowPower = false;
    this.devicePixelRatioSafe = 1;
    this.lodLevel = 'high';
    this.progressiveLoading = true;
    this.loadingProgress = 0;
    this.buildBudgetMs = 8;
    this.detailLevel = {
      archSegments: 24,
      archTessellation: 10,
      voussoirCount: 0,
      stoneTextureSize: 256,
      shadowMapSize: 2048,
      antialias: true,
    };

    this._animationId = null;
    this._boundAnimate = null;
    this._boundResize = null;
    this._boundMouseMove = null;
    this._boundClick = null;
    this._boundMouseLeave = null;
  }

  init(canvasId, options = {}) {
    if (typeof options === 'function') {
      options = { onClick: options };
    }

    this.canvas = document.getElementById(canvasId);
    this.tooltipEl = document.getElementById('viewerTooltip');
    this.hudEl = document.getElementById('hudInfo');
    this.minimapCanvas = document.getElementById('minimapCanvas');
    this.onClickCallback = options.onClick || null;
    this.onSegmentClick = options.onClick || null;

    this.detectDevice();

    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.scene = new THREE.Scene();
    this.scene.background = new THREE.Color(0x0f1a25);
    this.scene.fog = new THREE.Fog(0x0f1a25, 80, this.isLowPower ? 220 : 350);

    this.camera = new THREE.PerspectiveCamera(45, rect.width / rect.height, 0.1, 1000);
    this.camera.position.set(55, 40, 65);

    this.renderer = new THREE.WebGLRenderer({
      canvas: this.canvas,
      antialias: this.detailLevel.antialias,
      alpha: false,
      powerPreference: this.isLowPower ? 'low-power' : 'high-performance'
    });
    this.renderer.setPixelRatio(this.devicePixelRatioSafe);
    this.renderer.setSize(rect.width, rect.height);
    this.renderer.shadowMap.enabled = !this.isLowPower;
    this.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    if (this.renderer.capabilities.isWebGL2 === false) {
      this.renderer.shadowMap.enabled = false;
    }

    this.controls = new OrbitControls(this.camera, this.renderer.domElement);
    this.controls.enableDamping = !this.isLowPower;
    this.controls.dampingFactor = 0.08;
    this.controls.minDistance = 8;
    this.controls.maxDistance = 250;
    this.controls.maxPolarAngle = Math.PI / 2.05;

    this.setupLights();
    this.setupGround();

    this._boundResize = () => this.onResize();
    this._boundMouseMove = (e) => this.onMouseMove(e);
    this._boundClick = (e) => this.onClick(e);
    this._boundMouseLeave = () => this.onMouseLeave();

    window.addEventListener('resize', this._boundResize);
    this.canvas.addEventListener('mousemove', this._boundMouseMove);
    this.canvas.addEventListener('click', this._boundClick);
    this.canvas.addEventListener('mouseleave', this._boundMouseLeave);

    this._boundAnimate = () => this.animate();
    this.animate();
  }

  detectDevice() {
    const ua = navigator.userAgent || '';
    const w = window.innerWidth;
    const h = window.innerHeight;
    const md = w <= 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini|Mobi/i.test(ua);
    this.isMobile = md;
    this.isLowPower = md || (navigator.hardwareConcurrency && navigator.hardwareConcurrency <= 4) ||
      (navigator.deviceMemory && navigator.deviceMemory <= 2);
    this.devicePixelRatioSafe = Math.min(window.devicePixelRatio || 1, this.isLowPower ? 1.5 : 2);
    if (this.isLowPower) {
      this.lodLevel = 'low';
      this.detailLevel = {
        archSegments: 10,
        archTessellation: 5,
        voussoirCount: 0,
        stoneTextureSize: 128,
        shadowMapSize: 512,
        antialias: false,
      };
    } else if (this.isMobile) {
      this.lodLevel = 'medium';
      this.detailLevel = {
        archSegments: 16,
        archTessellation: 7,
        voussoirCount: Math.floor(8 / 2),
        stoneTextureSize: 128,
        shadowMapSize: 1024,
        antialias: w >= 768,
      };
    } else {
      this.lodLevel = 'high';
      this.detailLevel = {
        archSegments: 24,
        archTessellation: 10,
        voussoirCount: 0,
        stoneTextureSize: 256,
        shadowMapSize: 2048,
        antialias: true,
      };
    }
    if (this.detailLevel.voussoirCount === 0) {
      this.detailLevel.voussoirCount = this.lodLevel === 'high' ? -1 : 4;
    }
    console.log(`[Aqueduct3DViewer] Device: mobile=${this.isMobile}, lowPower=${this.isLowPower}, lod=${this.lodLevel}, dpr=${this.devicePixelRatioSafe}`);
  }

  setupLights() {
    const hemi = new THREE.HemisphereLight(0xe0d4a8, 0x1a2a3a, 0.6);
    this.scene.add(hemi);

    const sun = new THREE.DirectionalLight(0xffe4b5, 1.2);
    sun.position.set(40, 70, 30);
    sun.castShadow = !this.isLowPower;
    const smSize = this.detailLevel.shadowMapSize;
    sun.shadow.mapSize.set(smSize, smSize);
    sun.shadow.camera.near = 1;
    sun.shadow.camera.far = 200;
    sun.shadow.camera.left = -100;
    sun.shadow.camera.right = 100;
    sun.shadow.camera.top = 100;
    sun.shadow.camera.bottom = -100;
    this.scene.add(sun);

    if (!this.isLowPower) {
      const fill = new THREE.DirectionalLight(0x87ceeb, 0.35);
      fill.position.set(-40, 30, -30);
      this.scene.add(fill);
    }

    const ambient = new THREE.AmbientLight(0x404050, this.isLowPower ? 0.45 : 0.3);
    this.scene.add(ambient);
  }

  setupGround() {
    const size = 300;

    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 512;
    const ctx = canvas.getContext('2d');

    const grad = ctx.createLinearGradient(0, 0, 0, 512);
    grad.addColorStop(0, '#2a3a4a');
    grad.addColorStop(0.5, '#1e2d3c');
    grad.addColorStop(1, '#182030');
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, 512, 512);

    ctx.strokeStyle = 'rgba(201, 168, 73, 0.06)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 512; i += 16) {
      ctx.beginPath(); ctx.moveTo(i, 0); ctx.lineTo(i, 512); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(0, i); ctx.lineTo(512, i); ctx.stroke();
    }

    for (let i = 0; i < 80; i++) {
      ctx.fillStyle = `rgba(100, 120, 100, ${0.1 + Math.random() * 0.2})`;
      const x = Math.random() * 512;
      const y = Math.random() * 512;
      const r = 3 + Math.random() * 12;
      ctx.beginPath();
      ctx.ellipse(x, y, r * 1.5, r * 0.5, Math.random() * Math.PI, 0, Math.PI * 2);
      ctx.fill();
    }

    const groundTex = new THREE.CanvasTexture(canvas);
    groundTex.wrapS = groundTex.wrapT = THREE.RepeatWrapping;
    groundTex.repeat.set(4, 4);

    const ground = new THREE.Mesh(
      new THREE.PlaneGeometry(size, size),
      new THREE.MeshStandardMaterial({
        map: groundTex,
        roughness: 0.95,
        metalness: 0.0
      })
    );
    ground.rotation.x = -Math.PI / 2;
    ground.receiveShadow = true;
    this.scene.add(ground);

    const grid = new THREE.GridHelper(size, 60, 0x3d5a73, 0x253644);
    grid.material.opacity = 0.35;
    grid.material.transparent = true;
    grid.position.y = 0.01;
    this.scene.add(grid);
  }

  clearAqueduct() {
    this.lodObjects.forEach(lod => {
      lod.levels.forEach(l => {
        if (l.object && l.object.geometry) l.object.geometry.dispose();
      });
    });
    this.lodObjects = [];

    this.segmentMeshes.forEach(m => {
      this.scene.remove(m);
      if (m.geometry) m.geometry.dispose();
      if (m.material) {
        if (Array.isArray(m.material)) m.material.forEach(x => x.dispose());
        else m.material.dispose();
      }
    });
    this.segmentMeshes = [];
    this.segments.clear();
    this.selectedId = null;
    this.hoveredId = null;
    this.loadingProgress = 0;
  }

  updateLoadingProgress(done, total) {
    this.loadingProgress = done / total;
    const el = document.getElementById('loadingProgress');
    if (el) {
      el.style.width = (this.loadingProgress * 100).toFixed(0) + '%';
      el.parentElement.style.opacity = this.loadingProgress >= 1 ? '0' : '1';
    }
  }

  async buildAqueduct(aqueduct, segments) {
    this.clearAqueduct();

    const arches = segments.filter(s => s.segment_type === 'arch');
    const piers = segments.filter(s => s.segment_type === 'pier');

    const pierCount = piers.length || 20;
    const span = 6.0;
    const baseX = -((pierCount - 1) * span) / 2;

    const total = piers.length + arches.length + 1;
    let done = 0;

    const pierProxy = this.isMobile ? this.createPierProxy : null;

    for (let idx = 0; idx < piers.length; idx++) {
      const pier = piers[idx];
      const posX = baseX + idx * span;
      const height = pier.position_3d?.height || 15 + Math.random() * 10;

      let mesh;
      if (pierProxy && idx % 2 === 0 && this.progressiveLoading) {
        mesh = pierProxy.call(this, pier, posX, 0, 0, height);
      } else {
        mesh = this.createPierMesh(pier, posX, 0, 0, height);
      }
      mesh.userData = { id: pier.id, segment: pier };
      this.segmentMeshes.push(mesh);
      this.segments.set(pier.id, { mesh, data: pier });
      this.scene.add(mesh);

      done++;
      if (this.progressiveLoading && (idx % 5 === 4 || idx === piers.length - 1)) {
        this.updateLoadingProgress(done, total);
        await new Promise(r => setTimeout(r, 0));
      }
    }

    for (let idx = 0; idx < arches.length; idx++) {
      const arch = arches[idx];
      const posX = baseX + idx * span + span / 2;
      const height = (piers[idx]?.position_3d?.height || 18) - 3;
      const mesh = this.createArchMeshLOD(arch, posX, height, 0, span, span * 0.25);
      mesh.userData = { id: arch.id, segment: arch };
      this.segmentMeshes.push(mesh);
      this.segments.set(arch.id, { mesh, data: arch });
      this.scene.add(mesh);

      done++;
      if (this.progressiveLoading && (idx % 5 === 4 || idx === arches.length - 1)) {
        this.updateLoadingProgress(done, total);
        await new Promise(r => setTimeout(r, 0));
      }
    }

    const topLength = (pierCount - 1) * span;
    const topHeight = (piers[0]?.position_3d?.height || 18) + 2;
    const channel = this.createChannelMesh(baseX + topLength / 2, topHeight, 0, topLength);
    this.scene.add(channel);

    done++;
    this.updateLoadingProgress(done, total);

    if (this.progressiveLoading && pierProxy) {
      setTimeout(() => this.enhancePierDetails(piers, baseX, span), 800);
    }

    this.applyColorMode();
    this.zoomToFit();
  }

  enhancePierDetails(piers, baseX, span) {
    if (!piers || piers.length === 0) return;
    for (let idx = 0; idx < piers.length; idx++) {
      if (idx % 2 !== 0) continue;
      const pier = piers[idx];
      const existing = this.segments.get(pier.id);
      if (!existing || !existing.mesh.userData.isProxy) continue;
      const posX = baseX + idx * span;
      const height = pier.position_3d?.height || 15 + Math.random() * 10;
      const newMesh = this.createPierMesh(pier, posX, 0, 0, height);
      newMesh.userData = { id: pier.id, segment: pier };
      this.scene.remove(existing.mesh);
      existing.mesh.traverse(o => {
        if (o.geometry) o.geometry.dispose();
        if (o.material) {
          if (Array.isArray(o.material)) o.material.forEach(m => m.dispose());
          else o.material.dispose();
        }
      });
      const i = this.segmentMeshes.indexOf(existing.mesh);
      if (i >= 0) this.segmentMeshes[i] = newMesh;
      existing.mesh = newMesh;
      this.scene.add(newMesh);
    }
    this.applyColorMode();
  }

  createPierProxy(segment, x, y, z, height) {
    const group = new THREE.Group();
    const width = 1.2, depth = 2.5;
    const geo = new THREE.BoxGeometry(width, height, depth);
    const mat = new THREE.MeshStandardMaterial({
      color: 0x8a8070,
      roughness: 0.9,
      metalness: 0.03,
      wireframe: this.wireframe
    });
    const mesh = new THREE.Mesh(geo, mat);
    mesh.position.y = height / 2;
    mesh.castShadow = !this.isLowPower;
    mesh.receiveShadow = true;
    group.add(mesh);
    group.position.set(x, y, z);
    group.userData = segment;
    group.traverse(obj => { if (obj.isMesh) obj.userData = { id: segment.id, segment }; });
    group.userData.isProxy = true;
    return group;
  }

  createPierMesh(segment, x, y, z, height) {
    const group = new THREE.Group();

    const width = 1.2, depth = 2.5;

    const baseGeo = new THREE.BoxGeometry(width * 1.3, 1.5, depth * 1.2);
    const baseMat = this.createStoneMaterial(segment);
    const base = new THREE.Mesh(baseGeo, baseMat);
    base.position.y = 0.75;
    base.castShadow = true;
    base.receiveShadow = true;
    group.add(base);

    const bodyGeo = new THREE.BoxGeometry(width, height - 3, depth);
    const bodyMat = this.createStoneMaterial(segment);
    const body = new THREE.Mesh(bodyGeo, bodyMat);
    body.position.y = 1.5 + (height - 3) / 2;
    body.castShadow = true;
    body.receiveShadow = true;
    group.add(body);

    const capitalGeo = new THREE.BoxGeometry(width * 1.15, 1.0, depth * 1.1);
    const capitalMat = this.createStoneMaterial(segment);
    const capital = new THREE.Mesh(capitalGeo, capitalMat);
    capital.position.y = height - 1;
    capital.castShadow = true;
    capital.receiveShadow = true;
    group.add(capital);

    const impostGeo = new THREE.BoxGeometry(width * 1.08, 0.6, depth * 1.05);
    const impost = new THREE.Mesh(impostGeo, capitalMat);
    impost.position.y = height - 0.3;
    impost.castShadow = true;
    impost.receiveShadow = true;
    group.add(impost);

    group.position.set(x, y, z);
    group.userData = segment;
    group.traverse(obj => { if (obj.isMesh) obj.userData = { id: segment.id, segment }; });

    this.addStoneTexture(group);
    return group;
  }

  createArchMeshLOD(segment, x, y, z, span, rise) {
    if (!this.isMobile && !this.isLowPower) {
      return this.createArchMesh(segment, x, y, z, span, rise, 'high');
    }

    const lod = new LOD();

    const hi = this.createArchMesh(segment, x, y, z, span, rise, 'high');
    hi.visible = false;
    const hiGroup = new THREE.Group();
    hiGroup.add(...hi.children);
    hiGroup.position.copy(hi.position);
    lod.addLevel(hiGroup, 0);

    const me = this.createArchMesh(segment, x, y, z, span, rise, 'medium');
    const meGroup = new THREE.Group();
    meGroup.add(...me.children);
    meGroup.position.copy(me.position);
    lod.addLevel(meGroup, 45);

    const lo = this.createArchMesh(segment, x, y, z, span, rise, 'low');
    const loGroup = new THREE.Group();
    loGroup.add(...lo.children);
    loGroup.position.copy(lo.position);
    lod.addLevel(loGroup, 120);

    lod.position.set(x, y, z);

    const wrap = new THREE.Group();
    wrap.add(lod);
    wrap.userData = segment;
    wrap.position.set(x, y, z);

    wrap.traverse(obj => {
      if (obj.isMesh) {
        obj.userData = { id: segment.id, segment };
        this.segmentMeshes.push(obj);
      }
    });
    lod.traverse(obj => {
      if (obj.isMesh) obj.userData = { id: segment.id, segment };
    });

    lod.userData = segment;
    wrap.userData = { id: segment.id, segment, lod };
    this.lodObjects.push(lod);

    const dummy = new THREE.Group();
    dummy.position.set(x, y, z);
    dummy.userData = { id: segment.id, segment, lod, lodWrap: wrap };
    dummy.add(lod);

    return dummy;
  }

  createArchMesh(segment, x, y, z, span, rise, quality) {
    const group = new THREE.Group();
    const q = quality || this.lodLevel;

    let segCount = this.detailLevel.archSegments;
    let tess = this.detailLevel.archTessellation;
    let voussoirLim = this.detailLevel.voussoirCount;
    if (q === 'low') { segCount = 8; tess = 4; voussoirLim = 0; }
    else if (q === 'medium') { segCount = 14; tess = 6; voussoirLim = 4; }

    const curvePts = [];
    for (let i = 0; i <= segCount; i++) {
      const t = i / segCount;
      const px = -span / 2 + t * span;
      const pz = 4 * rise / (span * span) * (span * t) * (span - span * t);
      curvePts.push(new THREE.Vector3(px, pz, 0));
    }
    const curve = new THREE.CatmullRomCurve3(curvePts);

    const ribThickness = 0.8;
    const archWidth = 3.0;

    const tubeSegs = Math.max(8, segCount * 2);
    const archGeo = new THREE.TubeGeometry(curve, q === 'low' ? Math.floor(tubeSegs / 2) : tubeSegs,
      ribThickness / 2, tess, false);
    const archScale = new THREE.Vector3(1, 1, archWidth / ribThickness * 0.8);
    archGeo.scale(archScale.x, archScale.y, archScale.z);
    const archMat = this.createStoneMaterial(segment);
    const archTube = new THREE.Mesh(archGeo, archMat);
    archTube.castShadow = q !== 'low';
    archTube.receiveShadow = true;
    group.add(archTube);

    if (voussoirLim !== 0) {
      let stoneCount = Math.floor(span / 0.9);
      if (voussoirLim > 0) {
        stoneCount = Math.min(stoneCount, voussoirLim);
      }
      for (let i = 0; i < stoneCount; i++) {
        const t = (i + 0.5) / stoneCount;
        const pos = curve.getPointAt(t);
        const tangent = curve.getTangentAt(t).normalize();

        const voussoirW = span / stoneCount * 0.95;
        const wDiv = q === 'low' ? 1 : 2;
        const hDiv = q === 'low' ? 1 : 2;
        const dDiv = q === 'low' ? 1 : 2;
        const voussoirGeo = new THREE.BoxGeometry(voussoirW, ribThickness * 0.9, archWidth * 0.85,
          wDiv, hDiv, dDiv);
        const voussoir = new THREE.Mesh(voussoirGeo, archMat.clone());
        voussoir.position.copy(pos);
        voussoir.position.y += rise / 4;
        voussoir.rotation.z = -Math.atan2(tangent.x, Math.sqrt(1 - tangent.x * tangent.x)) * 0.8;
        voussoir.castShadow = q !== 'low';
        voussoir.receiveShadow = true;
        group.add(voussoir);
      }
    }

    if (q !== 'low') {
      const ksDiv = q === 'medium' ? 1 : 2;
      const keystoneGeo = new THREE.BoxGeometry(0.7, ribThickness * 1.2, archWidth * 0.95, ksDiv, ksDiv, ksDiv);
      const keystone = new THREE.Mesh(keystoneGeo, archMat.clone());
      keystone.position.set(0, rise + 0.3, 0);
      keystone.castShadow = q !== 'low';
      keystone.receiveShadow = true;
      group.add(keystone);
    }

    group.position.set(x, y, z);
    group.userData = segment;
    group.traverse(obj => { if (obj.isMesh) obj.userData = { id: segment.id, segment }; });

    if (q === 'high') this.addStoneTexture(group);
    return group;
  }

  createChannelMesh(x, y, z, length) {
    const group = new THREE.Group();

    const bedGeo = new THREE.BoxGeometry(length + 1, 0.4, 3.0);
    const bedMat = new THREE.MeshStandardMaterial({
      color: 0x5a5448,
      roughness: 0.9,
      metalness: 0.05
    });
    const bed = new THREE.Mesh(bedGeo, bedMat);
    bed.position.y = 0;
    bed.castShadow = true;
    bed.receiveShadow = true;
    group.add(bed);

    const wallH = 1.0;
    const wallGeo = new THREE.BoxGeometry(length + 1, wallH, 0.25);
    const wallMat = new THREE.MeshStandardMaterial({
      color: 0x7a7062,
      roughness: 0.85,
      metalness: 0.05
    });
    const wallN = new THREE.Mesh(wallGeo, wallMat);
    wallN.position.set(0, wallH / 2, -1.4);
    wallN.castShadow = true;
    group.add(wallN);
    const wallS = new THREE.Mesh(wallGeo, wallMat.clone());
    wallS.position.set(0, wallH / 2, 1.4);
    wallS.castShadow = true;
    group.add(wallS);

    const waterCanvas = document.createElement('canvas');
    waterCanvas.width = 512; waterCanvas.height = 128;
    const wctx = waterCanvas.getContext('2d');
    const wgrad = wctx.createLinearGradient(0, 0, 0, 128);
    wgrad.addColorStop(0, 'rgba(100, 180, 220, 0.85)');
    wgrad.addColorStop(1, 'rgba(60, 120, 180, 0.9)');
    wctx.fillStyle = wgrad;
    wctx.fillRect(0, 0, 512, 128);
    for (let i = 0; i < 20; i++) {
      wctx.strokeStyle = `rgba(255,255,255,${0.05 + Math.random() * 0.1})`;
      wctx.lineWidth = 1;
      wctx.beginPath();
      const y = Math.random() * 128;
      wctx.moveTo(0, y);
      for (let x = 0; x < 512; x += 20) {
        wctx.quadraticCurveTo(x + 10, y + (Math.random() - 0.5) * 6, x + 20, y);
      }
      wctx.stroke();
    }
    const waterTex = new THREE.CanvasTexture(waterCanvas);
    waterTex.wrapS = THREE.RepeatWrapping;
    waterTex.repeat.set(10, 1);

    const waterGeo = new THREE.PlaneGeometry(length, 2.6);
    const waterMat = new THREE.MeshStandardMaterial({
      map: waterTex,
      transparent: true,
      opacity: 0.85,
      roughness: 0.05,
      metalness: 0.1
    });
    const water = new THREE.Mesh(waterGeo, waterMat);
    water.rotation.x = -Math.PI / 2;
    water.position.y = 0.25;
    group.add(water);

    group.position.set(x, y, z);
    return group;
  }

  createStoneMaterial(segment) {
    const texSize = this.detailLevel.stoneTextureSize;
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = texSize;
    const ctx = canvas.getContext('2d');

    ctx.fillStyle = '#8a8070';
    ctx.fillRect(0, 0, texSize, texSize);

    const spotCount = Math.floor(400 * (texSize / 256));
    const baseColors = ['#7a6f5e', '#857a68', '#968a76', '#6f6555'];
    for (let i = 0; i < spotCount; i++) {
      ctx.fillStyle = baseColors[Math.floor(Math.random() * baseColors.length)];
      ctx.globalAlpha = 0.15 + Math.random() * 0.35;
      const x = Math.random() * texSize, y = Math.random() * texSize;
      const r = 3 + Math.random() * (20 * texSize / 256);
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;

    if (texSize >= 192) {
      ctx.strokeStyle = 'rgba(50, 40, 30, 0.25)';
      ctx.lineWidth = 1;
      const gapX = Math.floor(42 * texSize / 256);
      const gapY = Math.floor(36 * texSize / 256);
      for (let x = 0; x < texSize; x += gapX) {
        ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, texSize); ctx.stroke();
      }
      for (let y = 0; y < texSize; y += gapY) {
        ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(texSize, y); ctx.stroke();
      }

      const crackCount = Math.floor(50 * texSize / 256);
      for (let i = 0; i < crackCount; i++) {
        ctx.strokeStyle = `rgba(100, 80, 60, ${0.05 + Math.random() * 0.15})`;
        ctx.lineWidth = 0.5;
        const sx = Math.random() * texSize, sy = Math.random() * texSize;
        ctx.beginPath();
        ctx.moveTo(sx, sy);
        ctx.lineTo(sx + (Math.random() - 0.5) * (30 * texSize / 256), sy + (Math.random() - 0.5) * (30 * texSize / 256));
        ctx.stroke();
      }
    }

    const tex = new THREE.CanvasTexture(canvas);
    tex.wrapS = tex.wrapT = THREE.RepeatWrapping;
    tex.repeat.set(1.5, 1);
    if (this.detailLevel.stoneTextureSize <= 128) {
      tex.minFilter = THREE.LinearFilter;
      tex.magFilter = THREE.LinearFilter;
      tex.generateMipmaps = false;
    }

    return new THREE.MeshStandardMaterial({
      map: tex,
      roughness: this.isLowPower ? 1.0 : 0.85,
      metalness: 0.04,
      color: 0xffffff,
      wireframe: this.wireframe
    });
  }

  addStoneTexture(group) {
    group.traverse(obj => {
      if (obj.isMesh && obj.material?.map) {
        obj.material.map.repeat.set(
          0.8 + Math.random() * 0.8,
          0.8 + Math.random() * 0.8
        );
        obj.material.color.setHSL(0.08 + Math.random() * 0.03, 0.12 + Math.random() * 0.05, 0.52 + Math.random() * 0.05);
        obj.material.needsUpdate = true;
      }
    });
  }

  weatheringColor(depth) {
    let t;
    if (depth < 2) t = 0.0;
    else if (depth < 5) t = 0.15;
    else if (depth < 10) t = 0.35;
    else if (depth < 20) t = 0.6;
    else if (depth < 40) t = 0.8;
    else t = 1.0;
    return this.interpolateColors([
      [76, 175, 80], [139, 195, 74], [255, 235, 59],
      [255, 152, 0], [244, 67, 54], [136, 14, 79]
    ], t);
  }

  safetyColor(capacityRatio) {
    let t;
    if (capacityRatio >= 0.8) t = 0;
    else if (capacityRatio >= 0.65) t = 0.33;
    else if (capacityRatio >= 0.5) t = 0.66;
    else t = 1;
    return this.interpolateColors([
      [76, 175, 80], [255, 235, 59], [255, 152, 0], [244, 67, 54]
    ], t);
  }

  interpolateColors(colors, t) {
    t = Math.max(0, Math.min(1, t));
    const scaled = t * (colors.length - 1);
    const idx = Math.floor(scaled);
    const frac = scaled - idx;
    const c1 = colors[idx];
    const c2 = colors[Math.min(idx + 1, colors.length - 1)];
    const r = Math.round(c1[0] + (c2[0] - c1[0]) * frac);
    const g = Math.round(c1[1] + (c2[1] - c1[1]) * frac);
    const b = Math.round(c1[2] + (c2[2] - c1[2]) * frac);
    return new THREE.Color(`rgb(${r},${g},${b})`);
  }

  applyColorMode() {
    this.segments.forEach(({ mesh, data }, id) => {
      let color;
      if (this.colorMode === 'weathering') {
        color = this.weatheringColor(data.weathering_depth || 5);
      } else {
        color = this.safetyColor(data.capacity_ratio !== undefined ? data.capacity_ratio : 0.85);
      }

      const factor = 0.6;
      mesh.traverse(obj => {
        if (obj.isMesh && obj.material) {
          const mats = Array.isArray(obj.material) ? obj.material : [obj.material];
          mats.forEach(m => {
            if (!m.originalColor) m.originalColor = m.color.clone();
            if (id === this.selectedId) {
              m.color.setRGB(
                0.2 + color.r * 0.3,
                0.85 + color.g * 0.15,
                0.9 + color.b * 0.1
              );
              m.emissive = new THREE.Color(0x1a6a9a);
              m.emissiveIntensity = 0.3;
            } else if (id === this.hoveredId) {
              m.color.setRGB(
                0.15 + color.r * 0.5,
                0.4 + color.g * 0.5,
                0.8 + color.b * 0.2
              );
              m.emissive = new THREE.Color(color).multiplyScalar(0.25);
              m.emissiveIntensity = 0.8;
            } else {
              m.color.setRGB(
                m.originalColor.r * (1 - factor) + color.r * factor,
                m.originalColor.g * (1 - factor) + color.g * factor,
                m.originalColor.b * (1 - factor) + color.b * factor
              );
              m.emissive = new THREE.Color(0x000000);
              m.emissiveIntensity = 0;
            }
            m.needsUpdate = true;
          });
        }
      });
    });
  }

  setColorMode(mode) {
    this.colorMode = mode;
    const label = document.getElementById('colorModeLabel');
    if (label) label.textContent = `颜色模式: ${this.colorMode === 'weathering' ? '风化深度' : '承载力比'}`;
    this.applyColorMode();
  }

  toggleColorMode() {
    this.setColorMode(this.colorMode === 'weathering' ? 'safety' : 'weathering');
  }

  setWireframe(enabled) {
    this.wireframe = enabled;
    this.segmentMeshes.forEach(m => {
      m.traverse(obj => {
        if (obj.isMesh && obj.material) {
          const mats = Array.isArray(obj.material) ? obj.material : [obj.material];
          mats.forEach(mat => { mat.wireframe = this.wireframe; });
        }
      });
    });
  }

  toggleWireframe() {
    this.setWireframe(!this.wireframe);
  }

  selectSegment(segmentId) {
    this.setSelected(segmentId);
  }

  setSelected(id) {
    this.selectedId = id;
    this.applyColorMode();
  }

  resetView() {
    this.camera.position.set(55, 40, 65);
    this.controls.target.set(0, 12, 0);
    this.controls.update();
  }

  zoomToFit() {
    if (this.segmentMeshes.length === 0) return;
    const box = new THREE.Box3();
    this.segmentMeshes.forEach(m => box.expandByObject(m));
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.y, size.z);
    const dist = maxDim / (2 * Math.tan(Math.PI * this.camera.fov / 360));
    this.controls.target.copy(center);
    this.camera.position.set(
      center.x + dist * 0.9,
      center.y + dist * 0.7,
      center.z + dist * 0.9
    );
    this.camera.near = Math.max(0.1, dist / 100);
    this.camera.far = Math.max(1000, dist * 10);
    this.camera.updateProjectionMatrix();
    this.controls.update();
  }

  getSegmentData(segmentId) {
    const entry = this.segments.get(segmentId);
    return entry ? entry.data : null;
  }

  getAllSegments() {
    const result = [];
    this.segments.forEach(({ data }, id) => {
      result.push({ id, ...data });
    });
    return result;
  }

  destroy() {
    if (this._boundResize) {
      window.removeEventListener('resize', this._boundResize);
      this._boundResize = null;
    }
    if (this.canvas && this._boundMouseMove) {
      this.canvas.removeEventListener('mousemove', this._boundMouseMove);
      this._boundMouseMove = null;
    }
    if (this.canvas && this._boundClick) {
      this.canvas.removeEventListener('click', this._boundClick);
      this._boundClick = null;
    }
    if (this.canvas && this._boundMouseLeave) {
      this.canvas.removeEventListener('mouseleave', this._boundMouseLeave);
      this._boundMouseLeave = null;
    }

    if (this._animationId) {
      cancelAnimationFrame(this._animationId);
      this._animationId = null;
    }
    this._boundAnimate = null;

    this.clearAqueduct();

    if (this.renderer) {
      this.renderer.dispose();
      this.renderer = null;
    }

    this.scene = null;
    this.camera = null;
    this.controls = null;
    this.canvas = null;
    this.minimapCanvas = null;
    this.tooltipEl = null;
    this.hudEl = null;
    this.onClickCallback = null;
    this.onSegmentClick = null;
  }

  onResize() {
    const rect = this.canvas.parentElement.getBoundingClientRect();
    this.camera.aspect = rect.width / rect.height;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(rect.width, rect.height);
  }

  updateMouse(e) {
    const rect = this.canvas.getBoundingClientRect();
    this.mouse.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
    this.mouse.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;
  }

  getIntersects() {
    this.raycaster.setFromCamera(this.mouse, this.camera);
    const meshes = [];
    this.segmentMeshes.forEach(g => g.traverse(o => { if (o.isMesh) meshes.push(o); }));
    return this.raycaster.intersectObjects(meshes, false);
  }

  onMouseMove(e) {
    this.updateMouse(e);
    const intersects = this.getIntersects();
    const containerRect = this.canvas.parentElement.getBoundingClientRect();

    if (intersects.length > 0) {
      const id = intersects[0].object.userData.id;
      if (id && id !== this.hoveredId) {
        this.hoveredId = id;
        this.applyColorMode();
      }
      if (id) {
        const data = this.segments.get(id)?.data;
        if (data) this.showTooltip(e.clientX - containerRect.left + 15, e.clientY - containerRect.top + 15, data);
        if (this.hudEl) this.updateHUD(data);
      }
      this.canvas.style.cursor = 'pointer';
    } else {
      if (this.hoveredId) {
        this.hoveredId = null;
        this.applyColorMode();
      }
      if (this.tooltipEl) this.tooltipEl.style.display = 'none';
      if (this.hudEl) this.hudEl.textContent = '悬停结构段查看详情';
      this.canvas.style.cursor = 'default';
    }

    this.renderMinimap();
  }

  onMouseLeave() {
    this.hoveredId = null;
    this.applyColorMode();
    if (this.tooltipEl) this.tooltipEl.style.display = 'none';
  }

  onClick(e) {
    this.updateMouse(e);
    const intersects = this.getIntersects();
    if (intersects.length > 0) {
      const id = intersects[0].object.userData.id;
      this.setSelected(id);
      if (this.onClickCallback) this.onClickCallback(id, this.segments.get(id)?.data);
      if (this.onSegmentClick) this.onSegmentClick(id, this.segments.get(id)?.data);
    }
  }

  showTooltip(x, y, data) {
    if (!this.tooltipEl) return;
    const typeLabel = data.segment_type === 'arch' ? '拱券' : data.segment_type === 'pier' ? '桥墩' : data.segment_type;
    const cap = data.capacity_ratio !== undefined ? (data.capacity_ratio * 100).toFixed(1) + '%' : '-';
    const weath = data.weathering_depth?.toFixed(2) || '-';
    const stress = data.current_stress?.toFixed(2) || '-';
    const safety = data.safety_level || 'SAFE';
    const safetyClass = fmtCapacity(safety);

    this.tooltipEl.innerHTML = `
      <h4>${typeLabel} #${data.segment_index} · ${safety}</h4>
      <div class="tt-row"><span>承载力比</span><span class="val ${safetyClass}">${cap}</span></div>
      <div class="tt-row"><span>风化深度</span><span class="val ${fmtWeath(data.weathering_depth)}">${weath} mm</span></div>
      <div class="tt-row"><span>当前应力</span><span class="val">${stress} MPa</span></div>
      <div class="tt-row"><span>沉降量</span><span class="val">${data.settlement_mm?.toFixed(2) || '-'} mm</span></div>
      <div class="tt-row"><span>设计强度</span><span class="val">${data.design_strength?.toFixed(2)} MPa</span></div>
    `;
    this.tooltipEl.style.display = 'block';
    this.tooltipEl.style.left = x + 'px';
    this.tooltipEl.style.top = y + 'px';
  }

  updateHUD(data) {
    if (!this.hudEl) return;
    const typeLabel = data.segment_type === 'arch' ? '拱券' : data.segment_type === 'pier' ? '桥墩' : data.segment_type;
    const cap = data.capacity_ratio !== undefined ? (data.capacity_ratio * 100).toFixed(1) + '%' : '-';
    this.hudEl.innerHTML = `<b>${typeLabel} #${data.segment_index}</b><br>承载力比: <span style="color:var(--accent)">${cap}</span>`;
  }

  renderMinimap() {
    if (!this.minimapCanvas) return;
    const ctx = this.minimapCanvas.getContext('2d');
    const w = this.minimapCanvas.width, h = this.minimapCanvas.height;

    ctx.fillStyle = 'rgba(15, 25, 35, 0.9)';
    ctx.fillRect(0, 0, w, h);

    let minX = Infinity, maxX = -Infinity, minZ = Infinity, maxZ = -Infinity;
    this.segments.forEach(({ mesh }) => {
      minX = Math.min(minX, mesh.position.x);
      maxX = Math.max(maxX, mesh.position.x);
      minZ = Math.min(minZ, mesh.position.z);
      maxZ = Math.max(maxZ, mesh.position.z);
    });
    if (!isFinite(minX)) return;
    const pad = 15;
    const sx = (w - pad * 2) / (maxX - minX || 1);
    const sz = (h - pad * 2) / (maxZ - minZ || 1);

    const toX = (x) => pad + (x - minX) * sx;
    const toZ = (z) => pad + (z - minZ) * sz;

    ctx.strokeStyle = 'rgba(201, 168, 73, 0.25)';
    ctx.lineWidth = 1;
    ctx.setLineDash([2, 3]);

    const pierXs = [];
    this.segments.forEach(({ mesh, data }) => {
      if (data.segment_type === 'pier') pierXs.push(mesh.position.x);
    });
    pierXs.sort((a, b) => a - b);
    if (pierXs.length >= 2) {
      ctx.beginPath();
      ctx.moveTo(toX(pierXs[0]), h / 2);
      pierXs.forEach(x => ctx.lineTo(toX(x), h / 2));
      ctx.stroke();
    }
    ctx.setLineDash([]);

    this.segments.forEach(({ mesh, data }, id) => {
      const x = toX(mesh.position.x);
      const z = toZ(mesh.position.z);

      let color;
      if (this.colorMode === 'weathering') {
        color = data.weathering_depth;
        if (color < 2) ctx.fillStyle = '#4CAF50';
        else if (color < 5) ctx.fillStyle = '#8BC34A';
        else if (color < 10) ctx.fillStyle = '#FFEB3B';
        else if (color < 20) ctx.fillStyle = '#FF9800';
        else if (color < 40) ctx.fillStyle = '#F44336';
        else ctx.fillStyle = '#880E4F';
      } else {
        color = data.capacity_ratio;
        if (color >= 0.8) ctx.fillStyle = '#4CAF50';
        else if (color >= 0.65) ctx.fillStyle = '#FFEB3B';
        else if (color >= 0.5) ctx.fillStyle = '#FF9800';
        else ctx.fillStyle = '#F44336';
      }

      if (id === this.selectedId) {
        ctx.fillStyle = '#c9a849';
        ctx.beginPath(); ctx.arc(x, z, 7, 0, Math.PI * 2); ctx.fill();
        ctx.strokeStyle = '#fff'; ctx.lineWidth = 1.5; ctx.stroke();
      } else if (id === this.hoveredId) {
        ctx.fillStyle = '#fff';
        ctx.beginPath(); ctx.arc(x, z, 6, 0, Math.PI * 2); ctx.fill();
      } else {
        const r = data.segment_type === 'pier' ? 4 : 3;
        if (data.segment_type === 'pier') {
          ctx.fillRect(x - r, z - r * 1.5, r * 2, r * 3);
        } else {
          ctx.beginPath(); ctx.arc(x, z, r, 0, Math.PI * 2); ctx.fill();
        }
      }
    });

    const camDir = new THREE.Vector3();
    this.camera.getWorldDirection(camDir);
    const cx = toX(this.controls.target.x);
    const cz = toZ(this.controls.target.z);
    const angle = Math.atan2(camDir.x, camDir.z);
    ctx.save();
    ctx.translate(cx, cz);
    ctx.rotate(-angle);
    ctx.fillStyle = 'rgba(201, 168, 73, 0.6)';
    ctx.beginPath();
    ctx.moveTo(0, -14);
    ctx.lineTo(7, 8);
    ctx.lineTo(0, 3);
    ctx.lineTo(-7, 8);
    ctx.closePath();
    ctx.fill();
    ctx.restore();
  }

  animate() {
    this._animationId = requestAnimationFrame(this._boundAnimate);
    this.controls.update();

    if (this.lodObjects.length > 0 && this.camera) {
      for (let i = 0; i < this.lodObjects.length; i++) {
        this.lodObjects[i].update(this.camera);
      }
    }

    this.renderer.render(this.scene, this.camera);
  }
}

function fmtCapacity(s) {
  if (s === 'SAFE') return 'good';
  if (s === 'WARNING') return 'warn';
  if (s === 'DANGER') return 'bad';
  return 'crit';
}
function fmtWeath(d) {
  if (!d) return 'good';
  if (d < 5) return 'good';
  if (d < 10) return 'warn';
  if (d < 20) return 'bad';
  return 'crit';
}

export default Aqueduct3DViewer;
