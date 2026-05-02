'use strict';

const MAX_HISTORY = 4;
const MAX_LINE_CHARS = 220;

const H_COMPACT      = 260;
const H_WITH_SRC     = 480;
const H_SETTINGS     = 420;
const H_MODEL_SETUP  = 520;

const state = {
  running:       false,
  showSource:    false,
  settingsOpen:  false,
  modelSetupOpen: false,
  modelSetupReturn: 'main',
  currentModelName: '',
  srcTokens:     '',
  tgtTokens:     '',
};

const $ = id => document.getElementById(id);

let mainView, settingsView, modelSetupView;
let translationContent, sourceContent, statusDot, statusText, errorText;
let btnStart, btnToggleSrc, btnSettings, btnClose, btnCloseSettings, btnSaveSettings;
let sourcePanel, panelDivider, ollamaModelSelect, ollamaModelInput;
let btnManageModels;
let sbSrcLang, sbTgtLang;

function initDOM() {
  mainView           = $('main-view');
  settingsView       = $('settings-view');
  modelSetupView     = $('model-setup-view');
  translationContent = $('translation-text');
  sourceContent      = $('source-text');
  statusDot          = $('status-dot');
  statusText         = $('status-text');
  errorText          = $('error-text');
  btnStart           = $('btn-start');
  btnToggleSrc       = $('btn-toggle-source');
  btnSettings        = $('btn-settings');
  btnClose           = $('btn-close');
  btnCloseSettings   = $('btn-close-settings');
  btnSaveSettings    = $('btn-save-settings');
  btnManageModels    = $('btn-manage-models');
  sourcePanel        = $('source-panel');
  panelDivider       = $('panel-divider');
  ollamaModelSelect  = $('cfg-ollama-model-select');
  ollamaModelInput   = $('cfg-ollama-model');
  sbSrcLang          = $('sb-src-lang');
  sbTgtLang          = $('sb-tgt-lang');
}

function currentLine(container) {
  let el = container.querySelector('.line.current');
  if (!el) {
    el = document.createElement('div');
    el.className = 'line current cursor';
    container.appendChild(el);
  }
  return el;
}

function flushPaint(container) {
  // Force WebKitGTK to flush stale compositor texture caches after DOM changes.
  void container.offsetHeight;
}

function commitLine(container) {
  const cur = container.querySelector('.line.current');
  if (!cur || !cur.textContent.trim()) { cur?.remove(); return; }
  if (cur.textContent.length > MAX_LINE_CHARS) {
    cur.textContent = cur.textContent.slice(0, MAX_LINE_CHARS).trimEnd() + '…';
  }
  container.querySelectorAll('.line.history').forEach(el => {
    const age = parseInt(el.dataset.age || '1', 10);
    if (age >= MAX_HISTORY) { el.remove(); }
    else { el.dataset.age = String(age + 1); }
  });
  cur.classList.remove('current', 'cursor');
  cur.classList.add('history');
  cur.dataset.age = '1';
  flushPaint(container);
}

function updateCurrent(container, text) {
  const el = currentLine(container);
  el.textContent = text;
  el.classList.add('cursor');
}

function clearCurrent(container) {
  container.querySelector('.line.current')?.remove();
}

function onStatus({ status }) {
  statusDot.className = 'status-dot ' + status;
  statusText.textContent = {
    listening:    'Listening…',
    transcribing: 'Transcribing…',
    translating:  'Translating…',
    stopped:      'Stopped',
  }[status] ?? status;
  errorText.classList.add('hidden');
}

function onSegment({ text }) {
  state.srcTokens = state.srcTokens ? state.srcTokens + ' ' + text : text;
  if (state.showSource) updateCurrent(sourceContent, state.srcTokens);
}

function onSource({ text }) {
  state.srcTokens = text;
  if (state.showSource) updateCurrent(sourceContent, text);
}

function onToken({ token }) {
  state.tgtTokens += token;
  updateCurrent(translationContent, state.tgtTokens);
}

function onDone() {
  commitLine(translationContent);
  if (state.showSource) commitLine(sourceContent);
  else clearCurrent(sourceContent);
  state.srcTokens = '';
  state.tgtTokens = '';
}

function onError({ message }) {
  errorText.textContent = message;
  errorText.classList.remove('hidden');
  statusDot.className = 'status-dot error';
  state.srcTokens = '';
  state.tgtTokens = '';
  clearCurrent(translationContent);
  clearCurrent(sourceContent);
}

function setupEvents() {
  const rt = window.runtime;
  rt.EventsOn('pipeline:status',     onStatus);
  rt.EventsOn('translation:segment', onSegment);
  rt.EventsOn('translation:source',  onSource);
  rt.EventsOn('translation:token',   onToken);
  rt.EventsOn('translation:done',    onDone);
  rt.EventsOn('translation:error',   onError);
  rt.EventsOn('model:progress',      onModelProgress);
  rt.EventsOn('model:done',          onModelDone);
  rt.EventsOn('model:error',         onModelError);
}

// ── Model setup ───────────────────────────────────────────────────────────────

async function openModelSetup(returnTo = 'main') {
  state.modelSetupReturn = returnTo;
  state.modelSetupOpen = true;
  mainView.classList.add('hidden');
  settingsView.classList.add('hidden');
  modelSetupView.classList.remove('hidden');
  window.runtime.WindowSetSize(900, H_MODEL_SETUP);

  const models = await window.go.main.App.GetAvailableModels();
  const cfg = await window.go.main.App.GetConfig();
  state.currentModelName = (cfg.ModelPath || '').split(/[\\/]/).pop() || '';
  renderModelList(models, cfg.ModelPath || '');
}

function closeModelSetup() {
  state.modelSetupOpen = false;
  modelSetupView.classList.add('hidden');
  if (state.modelSetupReturn === 'settings') {
    settingsView.classList.remove('hidden');
    window.runtime.WindowSetSize(900, H_SETTINGS);
    return;
  }
  mainView.classList.remove('hidden');
  const h = state.showSource ? H_WITH_SRC : H_COMPACT;
  window.runtime.WindowSetSize(900, h);
}

function renderModelList(models, currentModelPath) {
  const list = $('model-list');
  list.innerHTML = '';
  const currentName = (currentModelPath || '').split(/[\\/]/).pop();
  for (const m of models) {
    const isCurrent = m.downloaded && currentName === m.name;
    const row = document.createElement('div');
    row.className = isCurrent ? 'model-option active-model' : 'model-option';
    row.innerHTML = `
      <div class="model-info">
        <span class="model-name">${m.display_name}</span>
        ${m.recommended ? '<span class="model-badge">Recommended</span>' : ''}
        <span class="model-desc">${m.description} &middot; ${m.size}</span>
      </div>
      <button class="btn-primary model-dl-btn" data-name="${m.name}">
        ${m.downloaded ? (isCurrent ? 'Selected' : 'Use') : 'Download'}
      </button>`;
    list.appendChild(row);
    const btn = row.querySelector('.model-dl-btn');
    btn.addEventListener('click', () => onModelAction(m));
  }
}

async function onModelAction(model) {
  if (model.downloaded) {
    if (state.currentModelName === model.name) {
      closeModelSetup();
      return;
    }
    await window.go.main.App.UseDownloadedModel(model.name);
    state.currentModelName = model.name;
    await loadSettings();
    closeModelSetup();
    return;
  }
  // Start download — disable all buttons and show progress.
  $('model-list').querySelectorAll('.btn-primary').forEach(b => b.disabled = true);
  const area = $('model-progress-area');
  area.classList.remove('hidden');
  $('model-progress-name').textContent = `Downloading ${model.display_name}…`;
  $('progress-fill').style.width = '0%';
  $('model-progress-text').textContent = '0%';
  window.go.main.App.DownloadModel(model.name);
}

function onModelProgress({ name, received, total }) {
  const pct = total > 0 ? Math.round((received / total) * 100) : 0;
  $('progress-fill').style.width = pct + '%';
  $('model-progress-text').textContent = pct + '%';
}

function onModelDone({ name, path }) {
  state.currentModelName = (path || '').split(/[\\/]/).pop() || state.currentModelName;
  loadSettings();
  closeModelSetup();
}

function onModelError({ message }) {
  $('model-progress-area').classList.add('hidden');
  $('model-list').querySelectorAll('.btn-primary').forEach(b => b.disabled = false);
  onError({ message: 'Model download failed: ' + message });
  closeModelSetup();
}

function openSettings() {
  state.settingsOpen = true;
  mainView.classList.add('hidden');
  settingsView.classList.remove('hidden');
  window.runtime.WindowSetSize(900, H_SETTINGS);
}

function closeSettings() {
  state.settingsOpen = false;
  settingsView.classList.add('hidden');
  mainView.classList.remove('hidden');
  const h = state.showSource ? H_WITH_SRC : H_COMPACT;
  window.runtime.WindowSetSize(900, h);
}

async function saveSettings() {
  try {
    const cfg = await window.go.main.App.GetConfig();
    cfg.Language        = $('cfg-language').value;
    cfg.TargetLanguage = $('cfg-target').value;
    const modelVal = ollamaModelSelect.value;
    cfg.OllamaModel = modelVal === '__custom__'
      ? (ollamaModelInput.value.trim() || cfg.OllamaModel)
      : modelVal;
    const url = $('cfg-ollama-url').value.trim();
    if (url) cfg.OllamaUrl = url;
    await window.go.main.App.SaveConfig(cfg);
    setSelectValue('sb-src-lang', cfg.Language);
    setSelectValue('sb-tgt-lang', cfg.TargetLanguage);
    closeSettings();
  } catch (e) {
    alert('Failed to save: ' + e);
  }
}

async function loadSettings() {
  try {
    const cfg = await window.go.main.App.GetConfig();
    setSelectValue('cfg-language', cfg.Language ?? 'de');
    setSelectValue('cfg-target',   cfg.TargetLanguage ?? 'en');
    const model = cfg.OllamaModel ?? '';
    const known = [...ollamaModelSelect.options].find(o => o.value === model);
    if (known) {
      ollamaModelSelect.value = model;
      ollamaModelInput.classList.add('hidden');
      ollamaModelInput.value = '';
    } else {
      ollamaModelSelect.value = '__custom__';
      ollamaModelInput.classList.remove('hidden');
      ollamaModelInput.value = model;
    }
    $('cfg-model-path').value = cfg.ModelPath ?? '';
    $('cfg-ollama-url').value = cfg.OllamaURL ?? '';
  } catch (_) {}
}

function setSelectValue(id, value) {
  const el = $(id);
  const options = Array.from(el.options);
  const index = options.findIndex(o => o.value === value);
  if (index !== -1) {
    el.selectedIndex = index;
  }
}

async function onStatusLangChange() {
  try {
    const cfg = await window.go.main.App.GetConfig();
    cfg.Language        = sbSrcLang.value;
    cfg.TargetLanguage = sbTgtLang.value;
    await window.go.main.App.SaveConfig(cfg);
    setSelectValue('cfg-language', cfg.Language);
    setSelectValue('cfg-target',   cfg.TargetLanguage);
  } catch (e) {
    onError({ message: 'Could not save language change: ' + e });
  }
}

async function init() {
  initDOM();
  sbSrcLang.addEventListener('change', onStatusLangChange);
  sbTgtLang.addEventListener('change', onStatusLangChange);
  
  while (!window.runtime || !window.go) {
    await new Promise(r => setTimeout(r, 50));
  }
  setupEvents();
  try {
    const cfg = await window.go.main.App.GetConfig();
    setSelectValue('sb-src-lang', cfg.Language ?? 'de');
    setSelectValue('sb-tgt-lang', cfg.TargetLanguage ?? 'en');
  } catch (err) {
    console.error('init: failed to load config:', err);
  }

  // Show model setup panel if no valid model is configured.
  // openModelSetup() just switches the visible view; init() continues so all
  // button listeners are attached before the user finishes the setup flow.
  try {
    if (await window.go.main.App.NeedsModelSetup()) {
      await openModelSetup();
    }
  } catch (_) {}

  try {
    if (await window.go.main.App.IsRunning()) {
      state.running = true;
      btnStart.textContent = '■ Stop';
      btnStart.classList.add('running');
    }
  } catch (_) {}
  btnStart.addEventListener('click', async () => {
    if (state.running) {
      await window.go.main.App.StopPipeline();
      state.running = false;
      btnStart.textContent = '▶ Start';
      btnStart.classList.remove('running');
    } else {
      try {
        await window.go.main.App.StartPipeline();
        state.running = true;
        btnStart.textContent = '■ Stop';
        btnStart.classList.add('running');
      } catch (e) {
        onError({ message: String(e) });
      }
    }
  });
  btnToggleSrc.addEventListener('click', () => {
    state.showSource = !state.showSource;
    sourcePanel.classList.toggle('hidden', !state.showSource);
    panelDivider.classList.toggle('hidden', !state.showSource);
    btnToggleSrc.classList.toggle('active', state.showSource);
    const h = state.showSource ? H_WITH_SRC : H_COMPACT;
    window.runtime.WindowSetSize(900, h);
    if (!state.showSource) clearCurrent(sourceContent);
    else if (state.srcTokens) updateCurrent(sourceContent, state.srcTokens);
  });
  btnSettings.addEventListener('click', async () => {
    await loadSettings();
    openSettings();
  });
  btnManageModels.addEventListener('click', async () => {
    await openModelSetup('settings');
  });
  $('btn-cancel-download').addEventListener('click', () => {
    window.go.main.App.CancelModelDownload();
    $('model-progress-area').classList.add('hidden');
    $('model-list').querySelectorAll('.btn-primary').forEach(b => b.disabled = false);
  });
  $('btn-use-existing').addEventListener('click', async () => {
    const path = $('existing-model-path').value.trim();
    if (!path) return;
    const cfg = await window.go.main.App.GetConfig();
    cfg.ModelPath = path;
    await window.go.main.App.SaveConfig(cfg);
    state.currentModelName = path.split(/[\\/]/).pop() || state.currentModelName;
    await loadSettings();
    closeModelSetup();
  });
  btnClose.addEventListener('click', () => window.runtime.Quit());
  btnCloseSettings.addEventListener('click', closeSettings);
  btnSaveSettings.addEventListener('click',  saveSettings);
  ollamaModelSelect.addEventListener('change', () => {
    const isCustom = ollamaModelSelect.value === '__custom__';
    ollamaModelInput.classList.toggle('hidden', !isCustom);
    if (isCustom) ollamaModelInput.focus();
  });
}

document.addEventListener('DOMContentLoaded', init);
