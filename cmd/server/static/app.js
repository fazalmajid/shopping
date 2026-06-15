'use strict';

// ── State ──────────────────────────────────────────────────────────────────
const state = {
  items: {},     // id → item object
  sections: [],  // [{id, name, sort_order}] sorted by sort_order
};

// SSE events that arrive before initial load are queued here.
let sseQueue = [];
let appReady = false;

// ── View switching ─────────────────────────────────────────────────────────
function showView(id) {
  document.querySelectorAll('.view').forEach(el => el.classList.remove('active'));
  document.getElementById(id).classList.add('active');
}

// ── WebAuthn helpers ───────────────────────────────────────────────────────
function bufferToBase64url(buf) {
  const bytes = new Uint8Array(buf instanceof ArrayBuffer ? buf : buf.buffer);
  let str = '';
  for (const b of bytes) str += String.fromCharCode(b);
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

function base64urlToBuffer(b64) {
  const b64std = b64.replace(/-/g, '+').replace(/_/g, '/');
  const str = atob(b64std);
  const bytes = new Uint8Array(str.length);
  for (let i = 0; i < str.length; i++) bytes[i] = str.charCodeAt(i);
  return bytes.buffer;
}

function fixupCreationOptions(opts) {
  opts.challenge = base64urlToBuffer(opts.challenge);
  opts.user.id   = base64urlToBuffer(opts.user.id);
  if (opts.excludeCredentials) {
    opts.excludeCredentials = opts.excludeCredentials.map(c =>
      ({...c, id: base64urlToBuffer(c.id)}));
  }
  return opts;
}

function fixupRequestOptions(opts) {
  opts.challenge = base64urlToBuffer(opts.challenge);
  if (opts.allowCredentials) {
    opts.allowCredentials = opts.allowCredentials.map(c =>
      ({...c, id: base64urlToBuffer(c.id)}));
  }
  return opts;
}

function credentialToJSON(cred) {
  const r = cred.response;
  const obj = {
    id:    cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type:  cred.type,
    response: {},
  };
  if (r.clientDataJSON)    obj.response.clientDataJSON    = bufferToBase64url(r.clientDataJSON);
  if (r.attestationObject) obj.response.attestationObject = bufferToBase64url(r.attestationObject);
  if (r.authenticatorData) obj.response.authenticatorData = bufferToBase64url(r.authenticatorData);
  if (r.signature)         obj.response.signature         = bufferToBase64url(r.signature);
  if (r.userHandle)        obj.response.userHandle        = bufferToBase64url(r.userHandle);
  return obj;
}

// ── Auth flows ─────────────────────────────────────────────────────────────
function checkSecureContext(errorId) {
  if (!window.isSecureContext || !navigator.credentials) {
    setError(errorId,
      'Passkeys require a secure context. Access this app via https:// or from localhost.');
    return false;
  }
  return true;
}

async function doLogin(email) {
  setError('login-error', '');
  if (!checkSecureContext('login-error')) return;
  try {
    const beginRes = await fetch('/api/auth/login/begin', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email}),
    });
    if (!beginRes.ok) { setError('login-error', (await beginRes.json()).error); return; }

    const opts = await beginRes.json();
    opts.publicKey = fixupRequestOptions(opts.publicKey);

    const assertion = await navigator.credentials.get(opts);
    const finishRes = await fetch('/api/auth/login/finish', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(credentialToJSON(assertion)),
    });
    if (!finishRes.ok) { setError('login-error', (await finishRes.json()).error); return; }

    await loadApp();
  } catch (err) {
    setError('login-error', err.message || 'Login failed');
  }
}

async function doRegister(token, displayName) {
  setError('register-error', '');
  if (!checkSecureContext('register-error')) return;
  try {
    const beginRes = await fetch('/api/auth/register/begin', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({token, display_name: displayName}),
    });
    if (!beginRes.ok) { setError('register-error', (await beginRes.json()).error); return; }

    const opts = await beginRes.json();
    opts.publicKey = fixupCreationOptions(opts.publicKey);

    const credential = await navigator.credentials.create(opts);
    const finishRes = await fetch('/api/auth/register/finish?token=' + encodeURIComponent(token), {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(credentialToJSON(credential)),
    });
    if (!finishRes.ok) { setError('register-error', (await finishRes.json()).error); return; }

    await loadApp();
  } catch (err) {
    setError('register-error', err.message || 'Registration failed');
  }
}

// ── App bootstrap ──────────────────────────────────────────────────────────
async function loadApp() {
  // Open SSE before the initial fetch so we don't miss events that fire
  // between the two requests.
  openSSE();

  const res = await fetch('/api/items');
  if (res.status === 401) { showView('view-login'); return; }

  const data = await res.json();
  state.sections = (data.sections || []).sort((a, b) => a.sort_order - b.sort_order);
  state.items = {};
  (data.items || []).forEach(it => { state.items[it.id] = it; });

  rebuildSectionSelect();
  renderAll();
  showView('view-app');

  // Drain events that arrived before this fetch completed.
  appReady = true;
  sseQueue.forEach(handleSSEEvent);
  sseQueue = [];
}

// ── SSE ────────────────────────────────────────────────────────────────────
let eventSource = null;

function openSSE() {
  if (eventSource) return;
  eventSource = new EventSource('/api/events');

  const SSE_EVENTS = [
    'item_added', 'item_classified', 'item_checked',
    'item_deleted', 'items_cleared', 'section_added',
  ];
  SSE_EVENTS.forEach(type => {
    eventSource.addEventListener(type, e => {
      const ev = {type, data: JSON.parse(e.data)};
      if (appReady) handleSSEEvent(ev);
      else sseQueue.push(ev);
    });
  });

  eventSource.onerror = () => {
    eventSource.close();
    eventSource = null;
    setTimeout(openSSE, 3000);
  };
}

function handleSSEEvent({type, data}) {
  switch (type) {
    case 'item_added':
      state.items[data.id] = data;
      renderAll();
      break;

    case 'item_classified':
      if (state.items[data.id]) {
        state.items[data.id].section_id = data.section_id;
        renderAll();
      }
      break;

    case 'item_checked':
      if (state.items[data.id]) {
        state.items[data.id].checked = data.checked;
        animateChecked(data.id, data.checked);
      }
      break;

    case 'item_deleted':
      delete state.items[data.id];
      renderAll();
      break;

    case 'items_cleared':
      (data.deleted_ids || []).forEach(id => delete state.items[id]);
      renderAll();
      break;

    case 'section_added':
      state.sections.push(data);
      state.sections.sort((a, b) => a.sort_order - b.sort_order);
      rebuildSectionSelect();
      updateSectionModalList();
      renderAll();
      break;
  }
}

// ── Rendering ──────────────────────────────────────────────────────────────
function renderAll() {
  const container = document.getElementById('sections-container');
  container.innerHTML = '';

  // Items with null section_id go into a "Classifying…" virtual group.
  const classifying = Object.values(state.items)
    .filter(it => !it.checked && it.section_id == null);
  if (classifying.length) {
    container.appendChild(makeSection('🔄 Classifying…', classifying, 'classifying', true));
  }

  state.sections.forEach(sec => {
    const secItems = Object.values(state.items)
      .filter(it => !it.checked && it.section_id === sec.id)
      .sort((a, b) => new Date(a.added_at) - new Date(b.added_at));
    if (secItems.length === 0) return;
    container.appendChild(makeSection(sec.name, secItems, sec.id, false));
  });
}

function makeSection(name, items, id, isVirtual) {
  const details = document.createElement('details');
  details.open = true;
  details.dataset.sectionId = id;

  const summary = document.createElement('summary');
  summary.textContent = name;
  const badge = document.createElement('span');
  badge.className = 'section-count';
  badge.textContent = items.length;
  summary.appendChild(badge);
  details.appendChild(summary);

  const list = document.createElement('div');
  list.className = 'item-list';
  items.forEach(it => list.appendChild(makeItem(it, isVirtual)));
  details.appendChild(list);

  return details;
}

function makeItem(item, isClassifying) {
  const row = document.createElement('div');
  row.className = 'item' + (item.checked ? ' checked' : '');
  row.dataset.itemId = item.id;

  const cb = document.createElement('input');
  cb.type = 'checkbox';
  cb.checked = item.checked;
  cb.addEventListener('change', () => checkItem(item.id, cb.checked));
  row.appendChild(cb);

  const text = document.createElement('span');
  text.className = 'item-text';
  text.textContent = item.text;
  row.appendChild(text);

  if (isClassifying) {
    const lbl = document.createElement('span');
    lbl.className = 'classifying-label';
    lbl.textContent = 'classifying…';
    row.appendChild(lbl);
  }

  const del = document.createElement('button');
  del.className = 'btn-danger';
  del.textContent = '✕';
  del.title = 'Remove item';
  del.addEventListener('click', () => deleteItem(item.id));
  row.appendChild(del);

  return row;
}

function animateChecked(id, checked) {
  const row = document.querySelector(`.item[data-item-id="${id}"]`);
  if (!row) { renderAll(); return; }
  if (checked) {
    row.classList.add('checked');
    row.querySelector('input[type="checkbox"]').checked = true;
    // Fade out after a short pause so the strikethrough is visible, then
    // remove from DOM (state is kept until items_cleared SSE or reload).
    setTimeout(() => {
      row.classList.add('fading');
      row.addEventListener('transitionend', () => row.remove(), {once: true});
    }, 600);
  } else {
    row.classList.remove('checked', 'fading');
    row.querySelector('input[type="checkbox"]').checked = false;
  }
}

// ── Section select ─────────────────────────────────────────────────────────
function rebuildSectionSelect() {
  const sel = document.getElementById('new-item-section');
  sel.innerHTML = '<option value="">Auto-detect</option>';
  state.sections.forEach(s => {
    const opt = document.createElement('option');
    opt.value = s.id;
    opt.textContent = s.name;
    sel.appendChild(opt);
  });
}

// ── API calls ──────────────────────────────────────────────────────────────
async function addItem(text, sectionID) {
  const body = {text};
  if (sectionID) body.section_id = parseInt(sectionID, 10);
  await fetch('/api/items', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body),
  });
}

async function checkItem(id, checked) {
  await fetch(`/api/items/${id}/check`, {
    method: 'PATCH',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({checked}),
  });
}

async function deleteItem(id) {
  await fetch(`/api/items/${id}`, {method: 'DELETE'});
}

async function clearChecked() {
  await fetch('/api/items/checked', {method: 'DELETE'});
}

async function logout() {
  await fetch('/api/auth/logout', {method: 'POST'});
  location.reload();
}

// ── Sections modal ─────────────────────────────────────────────────────────
function updateSectionModalList() {
  const ul = document.getElementById('section-list-items');
  ul.innerHTML = '';
  state.sections.forEach(s => {
    const li = document.createElement('li');
    li.textContent = s.name;
    ul.appendChild(li);
  });
}

async function addSection(name) {
  const res = await fetch('/api/sections', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({name}),
  });
  if (!res.ok) {
    setError('section-error', (await res.json()).error || 'Failed to add section');
    return false;
  }
  setError('section-error', '');
  document.getElementById('new-section-name').value = '';
  return true;
}

// ── Helpers ────────────────────────────────────────────────────────────────
function setError(id, msg) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = msg;
  el.style.display = msg ? 'block' : 'none';
}

// ── DOMContentLoaded ───────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
  // Detect invite flow: /invite/<token>
  let inviteToken = null;
  if (location.pathname.startsWith('/invite/')) {
    inviteToken = location.pathname.replace('/invite/', '').split('/')[0];
  }

  if (inviteToken) {
    const res = await fetch(`/api/invite/${inviteToken}`);
    if (res.ok) {
      const {email} = await res.json();
      document.getElementById('register-email-display').textContent = `Registering as: ${email}`;
    } else {
      setError('register-error', 'This invitation link is invalid or has expired.');
    }
    showView('view-register');
  } else {
    await loadApp();
  }

  // ── Login ──
  document.getElementById('btn-login').addEventListener('click', async () => {
    const email = document.getElementById('login-email').value.trim();
    if (!email) { setError('login-error', 'Please enter your email.'); return; }
    await doLogin(email);
  });
  document.getElementById('login-email').addEventListener('keydown', e => {
    if (e.key === 'Enter') document.getElementById('btn-login').click();
  });

  // ── Register ──
  document.getElementById('btn-register').addEventListener('click', async () => {
    const dn = document.getElementById('register-displayname').value.trim();
    if (!inviteToken) { setError('register-error', 'Invalid invitation link.'); return; }
    await doRegister(inviteToken, dn);
  });

  // ── Add item ──
  document.getElementById('add-item-form').addEventListener('submit', async e => {
    e.preventDefault();
    const input = document.getElementById('new-item-text');
    const text = input.value.trim();
    if (!text) return;
    const sectionID = document.getElementById('new-item-section').value;
    input.value = '';
    await addItem(text, sectionID);
  });

  // ── Clear checked ──
  document.getElementById('btn-clear-checked').addEventListener('click', clearChecked);

  // ── Logout ──
  document.getElementById('btn-logout').addEventListener('click', logout);

  // ── Invite modal ──
  const inviteModal = document.getElementById('modal-invite');
  document.getElementById('btn-invite').addEventListener('click', () => {
    document.getElementById('invite-email').value = '';
    setError('invite-error', '');
    inviteModal.showModal();
  });
  document.getElementById('btn-cancel-invite').addEventListener('click', () => inviteModal.close());
  document.getElementById('btn-send-invite').addEventListener('click', async () => {
    const email = document.getElementById('invite-email').value.trim();
    if (!email) { setError('invite-error', 'Please enter an email.'); return; }
    const res = await fetch('/api/invite', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email}),
    });
    if (res.ok) {
      inviteModal.close();
    } else {
      setError('invite-error', (await res.json()).error || 'Failed to send invite');
    }
  });

  // ── Sections modal ──
  const sectionsModal = document.getElementById('modal-sections');
  document.getElementById('btn-manage-sections').addEventListener('click', () => {
    updateSectionModalList();
    setError('section-error', '');
    document.getElementById('new-section-name').value = '';
    sectionsModal.showModal();
  });
  document.getElementById('btn-close-sections').addEventListener('click', () => sectionsModal.close());
  document.getElementById('btn-add-section').addEventListener('click', async () => {
    const name = document.getElementById('new-section-name').value.trim();
    if (!name) { setError('section-error', 'Please enter a section name.'); return; }
    await addSection(name);
  });
  document.getElementById('new-section-name').addEventListener('keydown', e => {
    if (e.key === 'Enter') document.getElementById('btn-add-section').click();
  });
});
