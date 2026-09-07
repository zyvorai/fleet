// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

(() => {
  'use strict';

  const THEME_KEY = 'zyvor-fleet-theme';
  const SIDEBAR_KEY = 'zyvor-fleet-sidebar-collapsed';
  const LOGIN_SAVE_KEY = 'zyvor-fleet-saved-login';
  const loginState = { step: 'identify', email: '' };

  function currentTheme() {
    return localStorage.getItem(THEME_KEY) === 'dark' ? 'dark' : 'light';
  }
  function applyTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    document.querySelectorAll('.theme-toggle').forEach(btn => { btn.innerHTML = themeIcon(theme); });
  }
  function toggleTheme() {
    const next = currentTheme() === 'dark' ? 'light' : 'dark';
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
  }
  function themeIcon(theme) {
    return theme === 'dark'
      ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>'
      : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5Z"/></svg>';
  }
  function themeToggleBtn() {
    return `<button class="icon-btn theme-toggle" data-action="toggle-theme" aria-label="Toggle theme">${themeIcon(currentTheme())}</button>`;
  }
  applyTheme(currentTheme());

  const app = document.getElementById('app');
  const toastRoot = document.getElementById('toast-root');
  const state = { user: null, route: 'overview', sites: [], groups: [], revisions: [], rollouts: [], events: [], dashboard: null, integrations: [], tokens: [], users: [], apiTokens: [], webhooks: [], audit: [] };

  const icons = {
    overview:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></svg>',
    sites:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M4 7h16M4 17h16M7 4v6m10-6v6M7 14v6m10-6v6"/><circle cx="7" cy="7" r="3"/><circle cx="17" cy="17" r="3"/></svg>',
    rollouts:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M5 12h12m-4-4 4 4-4 4"/><path d="M5 5h5M5 19h5"/></svg>',
    events:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M4 5h16M4 12h16M4 19h10"/><circle cx="19" cy="19" r="2"/></svg>',
    settings:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1.03 1.56V21h-4v-.08A1.7 1.7 0 0 0 8.94 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.52-1H3v-4h.08A1.7 1.7 0 0 0 4.6 8.94a1.7 1.7 0 0 0-.34-1.88L4.2 7l2.83-2.83.06.06A1.7 1.7 0 0 0 8.97 4.6 1.7 1.7 0 0 0 10 3.08V3h4v.08a1.7 1.7 0 0 0 1.06 1.52 1.7 1.7 0 0 0 1.88-.34l.06-.06L19.83 7l-.06.06a1.7 1.7 0 0 0-.37 1.88A1.7 1.7 0 0 0 20.92 10H21v4h-.08A1.7 1.7 0 0 0 19.4 15Z"/></svg>',
    menu:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M4 7h16M4 12h16M4 17h16"/></svg>',
    refresh:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 8a7 7 0 0 1 11.5-1L20 12M4 12l2.4 5a7 7 0 0 0 11.5-1"/></svg>',
    collapse:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M15 5l-7 7 7 7M5 5v14"/></svg>',
    back:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M15 5l-7 7 7 7"/></svg>',
    eye:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7-11-7-11-7Z"/><circle cx="12" cy="12" r="3"/></svg>',
    eyeOff:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M3 3l18 18M10.6 10.6a3 3 0 0 0 4.2 4.2M9.9 5.1A11 11 0 0 1 12 5c7 0 11 7 11 7a13.2 13.2 0 0 1-3.2 3.9M6.6 6.6C3.9 8.3 2 12 2 12s4 7 11 7a10 10 0 0 0 4-.8"/></svg>',
    topology:'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M12 6.6v3M14.1 13.5l3.4 2M9.9 13.5l-3.4 2"/><circle cx="12" cy="4.6" r="1.8"/><circle cx="19.2" cy="17" r="1.8"/><circle cx="4.8" cy="17" r="1.8"/><circle cx="12" cy="11.5" r="2.9" fill="currentColor" stroke="none"/></svg>'
  };

  function logo() {
    return '<span class="brand-logo"><svg class="brand-z" viewBox="0 0 18 18" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linejoin="round" stroke-linecap="round"><path d="M2 2h14L6.6 16H16"/></svg><span class="brand-wordmark">zyvor</span></span>';
  }
  function sidebarCollapsed() { return localStorage.getItem(SIDEBAR_KEY) === '1'; }

  async function api(path, options = {}) {
    const opts = { credentials: 'same-origin', ...options, headers: { ...(options.headers || {}) } };
    if (opts.body && typeof opts.body !== 'string') {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(opts.body);
    }
    if (opts.method && !['GET','HEAD'].includes(opts.method.toUpperCase())) opts.headers['X-Zyvor-Request'] = '1';
    const res = await fetch(path, opts);
    let data = null;
    const text = await res.text();
    if (text) { try { data = JSON.parse(text); } catch { data = text; } }
    if (!res.ok) {
      const err = new Error(data?.error || `Request failed (${res.status})`);
      err.status = res.status;
      throw err;
    }
    // Empty Go slices serialize as JSON null, not []; every successful list
    // endpoint here is an array, so normalize once at the boundary instead of
    // guarding every call site.
    if (data === null) data = [];
    return data;
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[ch]));
  }
  function relativeTime(v) {
    if (!v) return 'never'; const d = new Date(v), sec = Math.floor((Date.now()-d.getTime())/1000);
    if (sec < 0) { const s=-sec; if (s < 60) return 'in <1m'; if (s < 3600) return `in ${Math.floor(s/60)}m`; if (s < 86400) return `in ${Math.floor(s/3600)}h`; return `in ${Math.floor(s/86400)}d`; }
    if (sec < 5) return 'now'; if (sec < 60) return `${sec}s ago`; if (sec < 3600) return `${Math.floor(sec/60)}m ago`; if (sec < 86400) return `${Math.floor(sec/3600)}h ago`; return `${Math.floor(sec/86400)}d ago`;
  }
  function memory(v) { if (!v) return '—'; const g=v/1073741824; return g>=1?`${g.toFixed(g>=10?0:1)} GB`:`${Math.round(v/1048576)} MB`; }
  function initial(name) { return String(name || 'Z').trim().slice(0,1).toUpperCase(); }
  function toast(message, kind='') { const el=document.createElement('div');el.className=`toast ${kind}`;el.textContent=message;toastRoot.appendChild(el);setTimeout(()=>el.remove(),4200); }
  function progress(ro) { const total=ro.siteIds?.length||0, done=ro.completedSites?.length||0; return total?Math.round(done/total*100):0; }
  function statusClass(s) { return ['online','degraded'].includes(s)?s:'offline'; }

  function loginView() {
    app.innerHTML = `
      <main class="login-page">
        ${themeToggleBtn().replace('class="icon-btn theme-toggle"', 'class="icon-btn theme-toggle login-theme-toggle"')}
        <div class="login-scroll">
          <section class="login-chapter login-chapter-hero">
            <div class="login-chapter-inner">
              <div class="brand">${logo()}<span class="brand-sub">Fleet</span></div>
              <p class="login-wordmark">Edge operations, without the dependency</p>
              <h1 class="login-hero-title">Every site.<br><span class="dim">Still running.</span></h1>
              <p class="login-tagline">Operate Linux, Kubernetes, containers and virtual machines across thousands of remote locations—even when the WAN disappears.</p>
              <div class="hero-proof">
                <div class="proof-item"><i class="proof-dot"></i> Offline-first autonomy</div>
                <div class="proof-item"><i class="proof-dot"></i> Pull-based secure control</div>
                <div class="proof-item"><i class="proof-dot"></i> Apache-2.0 core</div>
              </div>
              <div class="login-cta"><a class="primary-btn" href="#sign-in">Sign in</a></div>
              <p class="login-chapter-note">Scroll down, or tap Sign in, to continue.</p>
            </div>
          </section>
          <section class="login-chapter login-chapter-sign-in" id="sign-in">
            <div class="login-chapter-inner login-sign-in-inner" id="login-form-slot"></div>
          </section>
        </div>
      </main>`;
    const saved = localStorage.getItem(LOGIN_SAVE_KEY) || '';
    loginState.email = saved;
    loginState.step = saved ? 'password' : 'identify';
    // Don't autofocus on a fresh, unremembered load: focusing the email field
    // would drag the scroll-snap container straight to the sign-in chapter,
    // skipping the hero chapter this page is supposed to show first.
    renderLoginStep(Boolean(saved));
  }

  function renderLoginStep(autofocus) {
    const slot = document.getElementById('login-form-slot');
    if (!slot) return;
    if (autofocus === undefined) autofocus = true;
    if (loginState.step === 'identify') {
      slot.innerHTML = `
        <form class="login-card" id="login-form-identify">
          <h2 class="login-form-heading">Sign in to Zyvor Fleet</h2>
          <div class="field"><label for="email">Email</label><input id="email" name="email" type="email" autocomplete="username" placeholder="admin@zyvor.local" value="${escapeHTML(loginState.email)}" required></div>
          <button class="primary-btn full" type="submit">Continue</button>
          <div class="login-note">Session cookies are HttpOnly and same-site. No third-party scripts or fonts.</div>
        </form>`;
      if (autofocus) document.getElementById('email')?.focus();
    } else {
      slot.innerHTML = `
        <form class="login-card" id="login-form">
          <button type="button" class="login-identity-chip" data-action="login-back" aria-label="Change account">${icons.back}<span>${escapeHTML(loginState.email)}</span></button>
          <h2 class="login-form-heading">Enter your password</h2>
          <div class="field"><label for="password">Password</label>
            <div class="login-password-wrap">
              <input id="password" name="password" type="password" autocomplete="current-password" required>
              <button type="button" class="login-password-toggle" data-action="toggle-password" aria-label="Show password">${icons.eye}</button>
            </div>
          </div>
          <label class="login-remember"><input type="checkbox" id="remember-me" ${localStorage.getItem(LOGIN_SAVE_KEY)?'checked':''}> Remember me on this device</label>
          <div class="login-error" id="login-error"></div>
          <button class="primary-btn full" type="submit">Sign In</button>
          <div class="login-note">Session cookies are HttpOnly and same-site. No third-party scripts or fonts.</div>
        </form>`;
      if (autofocus) document.getElementById('password')?.focus();
    }
  }

  function shell() {
    const nav = [['overview','Overview'],['sites','Sites'],['rollouts','Rollouts'],['events','Events'],['settings','Settings']];
    app.innerHTML = `
      <div class="shell ${sidebarCollapsed()?'collapsed':''}">
        <div class="sidebar-backdrop" id="sidebar-backdrop" data-action="menu"></div>
        <aside class="sidebar" id="sidebar">
          <div class="brand">${logo()}<span class="brand-sub">Fleet</span></div>
          <nav class="nav">${nav.map(([id,label])=>`<button class="nav-btn ${state.route===id?'active':''}" data-route="${id}">${icons[id]}<span>${label}</span></button>`).join('')}</nav>
          <button class="nav-btn sidebar-collapse-btn" data-action="toggle-sidebar" aria-label="Collapse sidebar">${icons.collapse}<span>Collapse</span></button>
          <div class="sidebar-bottom"><div class="user-chip"><span class="avatar">${escapeHTML(initial(state.user.name))}</span><div class="user-lines"><strong>${escapeHTML(state.user.name||state.user.email)}</strong><span>${escapeHTML(state.user.role)}</span></div></div></div>
        </aside>
        <section class="main">
          <header class="topbar">
            <div class="top-actions"><button class="icon-btn mobile-menu" data-action="menu" aria-label="Menu">${icons.menu}</button><span class="crumb">Zyvor Platform / <strong>${escapeHTML(cap(state.route))}</strong></span></div>
            <div class="top-actions"><span class="live-pill"><i></i> Control plane live</span>${themeToggleBtn()}<button class="icon-btn" data-action="refresh" aria-label="Refresh">${icons.refresh}</button><button class="ghost-btn" data-action="logout">Sign out</button></div>
          </header>
          <main class="content" id="content"><div class="app-loading" style="min-height:45vh"><div class="spinner"></div></div></main>
        </section>
      </div>`;
  }

  function cap(s){return s.slice(0,1).toUpperCase()+s.slice(1)}

  async function navigate(route) {
    if (!['overview','sites','rollouts','events','settings'].includes(route)) route='overview';
    state.route=route; location.hash=route;
    document.querySelectorAll('.nav-btn').forEach(x=>x.classList.toggle('active',x.dataset.route===route));
    const crumb=document.querySelector('.crumb strong'); if(crumb)crumb.textContent=cap(route);
    const content=document.getElementById('content'); if(content)content.innerHTML='<div class="app-loading" style="min-height:45vh"><div class="spinner"></div></div>';
    try { await renderRoute(route); } catch(err) { if(err.status===401){state.user=null;loginView();return} toast(err.message,'error'); if(content) content.innerHTML=`<div class="empty"><strong>Could not load this view</strong>${escapeHTML(err.message)}</div>`; }
  }

  async function renderRoute(route) {
    if (route==='overview') return renderOverview();
    if (route==='sites') return renderSites();
    if (route==='rollouts') return renderRollouts();
    if (route==='events') return renderEvents();
    if (route==='settings') return renderSettings();
  }

  async function renderOverview() {
    const [dashboard,sites,rollouts] = await Promise.all([api('/api/v1/dashboard'),api('/api/v1/sites'),api('/api/v1/rollouts')]);
    state.dashboard=dashboard;state.sites=sites;state.rollouts=rollouts;
    const s=dashboard.sites;
    const content=document.getElementById('content');
    content.innerHTML=`
      <div class="page-head"><div><div class="page-kicker">Fleet pulse</div><h1>Good evening.</h1><p>${s.total?s.online+' of '+s.total+' sites are connected.':'Enroll your first site to bring the fleet online.'} Remote sites keep their last desired state when disconnected.</p></div><div class="actions"><button class="ghost-btn" data-action="enroll">Enroll site</button><button class="primary-btn" data-action="new-rollout">New rollout</button></div></div>
      <section class="metric-grid">
        ${metric('Sites',s.total,'registered edge locations','')}
        ${metric('Online',s.online, s.total?`${Math.round(s.online/s.total*100)}% connected`:'No sites yet','good')}
        ${metric('Autonomy',s.autonomy,'sites operating locally',s.autonomy?'warn':'')}
        ${metric('Rollouts',dashboard.runningRollouts,'currently progressing',dashboard.runningRollouts?'warn':'')}
      </section>
      <section class="dashboard-grid">
        <div class="card"><div class="card-pad card-head"><div><div class="card-title">Fleet topology</div><div class="card-sub">Live control-plane reachability</div></div><span class="tiny">${Object.keys(dashboard.regions||{}).length} regions</span></div>${fleetStage(sites)}</div>
        <div class="card card-pad"><div class="card-head"><div><div class="card-title">Operational stream</div><div class="card-sub">Latest fleet changes</div></div><button class="link-btn" data-route="events">View all</button></div>${eventList(dashboard.recentEvents||[])}</div>
      </section>`;
  }
  function metric(label,value,foot,cls){return `<article class="metric"><div class="metric-label">${escapeHTML(label)}</div><div class="metric-value ${cls}">${escapeHTML(value)}</div><div class="metric-foot">${escapeHTML(foot)}</div></article>`}
  function fleetStage(sites){ if(!sites.length)return `<div class="fleet-stage"><div class="hub">${icons.topology}</div><div class="pulse-ring"></div></div>`; const pts=sites.slice(0,28).map((site,i)=>{const angle=(i/sites.slice(0,28).length)*Math.PI*2-.8;const ring=i%3;const rx=[27,36,42][ring],ry=[29,37,42][ring];const x=50+Math.cos(angle)*rx,y=50+Math.sin(angle)*ry;return `<button class="site-dot ${statusClass(site.status)}" style="left:${x.toFixed(1)}%;top:${y.toFixed(1)}%" data-site="${escapeHTML(site.id)}" title="${escapeHTML(site.name)}"><i></i><span>${escapeHTML(site.name)}</span></button>`}).join('');return `<div class="fleet-stage"><div class="hub">${icons.topology}</div><div class="pulse-ring"></div>${pts}</div>`}
  function eventList(events){if(!events.length)return '<div class="empty"><strong>No fleet events yet</strong>Enrollment, reconnects and rollouts will appear here.</div>';return `<div class="event-list">${[...events].reverse().slice(0,8).map(e=>`<div class="event-row"><i class="event-bullet ${escapeHTML(e.severity)}"></i><div class="event-text"><strong>${escapeHTML(e.message)}</strong><small>${escapeHTML(e.kind)}${e.siteId?' · '+escapeHTML(e.siteId):''}</small></div><span class="event-time">${relativeTime(e.createdAt)}</span></div>`).join('')}</div>`}

  async function renderSites() {
    [state.sites,state.groups]=await Promise.all([api('/api/v1/sites'),api('/api/v1/site-groups')]); const content=document.getElementById('content');
    content.innerHTML=`<div class="page-head"><div><div class="page-kicker">Every location</div><h1>Sites</h1><p>One inventory for datacenter, branch, factory, retail and rugged edge systems. Group sites dynamically with labels for safer rollouts.</p></div><div class="actions"><button class="ghost-btn" data-action="new-group">New group</button><button class="primary-btn" data-action="enroll">Enroll site</button></div></div>
      <div class="section-title">Dynamic groups</div><section class="group-grid">${state.groups.length?state.groups.map(groupCard).join(''):'<div class="empty"><strong>No site groups yet</strong>Create a selector such as env=production or region=west.</div>'}</section>
      <div class="section-title">Fleet inventory</div>
      <div class="table-card"><div class="table-toolbar"><input class="search" id="site-search" placeholder="Search sites, regions, runtimes…" aria-label="Search sites"><span class="tiny">${state.sites.length} registered</span></div><div id="site-table">${siteTable(state.sites)}</div></div>`;
    document.getElementById('site-search')?.addEventListener('input',e=>{const q=e.target.value.toLowerCase();const filtered=state.sites.filter(s=>JSON.stringify(s).toLowerCase().includes(q));document.getElementById('site-table').innerHTML=siteTable(filtered)});
  }
  function groupCard(g){const ids=g.resolvedSiteIds||[];const selector=Object.entries(g.selector||{}).map(([k,v])=>`${escapeHTML(k)}=${escapeHTML(v)}`).join(' · ');const canDelete=['admin','operator'].includes(state.user.role);return `<article class="revision-card"><div class="card-head"><span class="tag">${ids.length} sites</span>${canDelete?`<button class="link-btn danger-link" data-action="delete-group" data-group="${escapeHTML(g.id)}">Remove</button>`:'<span class="tiny">dynamic</span>'}</div><h3>${escapeHTML(g.name)}</h3><p class="muted tiny">${escapeHTML(g.description||'Reusable rollout target')}</p><div class="cap-list">${selector?`<span class="tag mono">${selector}</span>`:''}${(g.siteIds||[]).length?`<span class="tag">${g.siteIds.length} pinned</span>`:''}</div></article>`}
  function siteTable(sites){if(!sites.length)return '<div class="empty"><strong>No matching sites</strong>Create an enrollment token and start fleet-agent on an edge host.</div>';return `<table><thead><tr><th>Site</th><th>Status</th><th>Runtime</th><th>Compute</th><th>Desired</th><th>Last seen</th><th></th></tr></thead><tbody>${sites.map(s=>`<tr><td><div class="site-name"><span class="avatar">${escapeHTML(initial(s.name))}</span><div>${escapeHTML(s.name)}<div class="tiny">${escapeHTML(s.region||'Unassigned region')}</div></div></div></td><td><span class="status ${statusClass(s.status)}"><i></i>${escapeHTML(s.status)}</span>${s.autonomyMode?'<span class="tag">autonomy</span>':''}${s.maintenance?'<span class="tag danger-tag">maintenance</span>':''}</td><td>${(s.inventory?.runtimes||[]).slice(0,3).map(x=>`<span class="tag">${escapeHTML(x)}</span>`).join('')||'—'}</td><td>${escapeHTML(s.inventory?.cpuCount||'—')} CPU · ${memory(s.inventory?.memoryBytes)}</td><td><span class="mono">${escapeHTML(shortID(s.desiredRevision)||'—')}</span></td><td>${relativeTime(s.lastSeen)}</td><td><button class="link-btn" data-site="${escapeHTML(s.id)}">Details</button></td></tr>`).join('')}</tbody></table>`}

  async function renderRollouts(){const [revisions,rollouts,sites,groups]=await Promise.all([api('/api/v1/revisions'),api('/api/v1/rollouts'),api('/api/v1/sites'),api('/api/v1/site-groups')]);state.revisions=revisions;state.rollouts=rollouts;state.sites=sites;state.groups=groups;const content=document.getElementById('content');content.innerHTML=`<div class="page-head"><div><div class="page-kicker">Desired state</div><h1>Rollouts</h1><p>Promote one revision in strict waves with approvals, health gates, maintenance windows and rollback protection.</p></div><div class="actions"><button class="ghost-btn" data-action="new-revision">New revision</button><button class="primary-btn" data-action="new-rollout">Start rollout</button></div></div>
      <div class="section-title">Active & recent</div><section class="rollout-grid">${rollouts.length?rollouts.slice().reverse().map(rolloutCard).join(''):'<div class="empty"><strong>No rollouts yet</strong>Create a revision, then promote it across selected sites.</div>'}</section>
      <div class="section-title">Revisions</div><section class="revision-grid">${revisions.length?revisions.slice().reverse().map(revisionCard).join(''):'<div class="empty"><strong>No desired-state revisions</strong>A revision can contain systemd, container, K3s and QEMU workload declarations.</div>'}</section>`}
  function rolloutCard(r){const p=progress(r);const failed=r.failedSites?.length||0;const controls=[];if(['running','scheduled'].includes(r.status))controls.push(`<button class="link-btn" data-action="rollout-pause" data-rollout="${escapeHTML(r.id)}">Pause</button>`);if(r.status==='paused')controls.push(`<button class="link-btn" data-action="rollout-resume" data-rollout="${escapeHTML(r.id)}">Resume</button>`);if(r.status==='pending_approval'&&state.user.role==='admin')controls.push(`<button class="link-btn" data-action="rollout-approve" data-rollout="${escapeHTML(r.id)}">Approve</button>`);if(r.status==='failed')controls.push(`<button class="link-btn" data-action="rollout-retry" data-rollout="${escapeHTML(r.id)}">Retry</button>`);if(!['completed','completed_with_failures','failed','aborted','rolled_back','expired'].includes(r.status))controls.push(`<button class="link-btn danger-link" data-action="rollout-abort" data-rollout="${escapeHTML(r.id)}">Abort</button>`);const canRollback=r.previousRevisions&&Object.keys(r.previousRevisions).length&&Object.values(r.previousRevisions).every(Boolean);if(canRollback)controls.push(`<button class="link-btn" data-action="rollout-rollback" data-rollout="${escapeHTML(r.id)}">Rollback</button>`);return `<article class="rollout-card"><div class="card-head"><span class="tag">${escapeHTML(r.status.replaceAll('_',' '))}</span><span class="tiny">wave ${r.currentWave||0} · max failures ${r.maxFailures??0}</span></div><h3>${escapeHTML(r.name)}</h3><div class="muted tiny mono">${escapeHTML(shortID(r.revisionId))}</div><div class="cap-list rollout-flags">${r.autoRollback?'<span class="tag">auto rollback</span>':''}${r.approvalRequired?'<span class="tag">approval</span>':''}${r.pauseSeconds?`<span class="tag">${r.pauseSeconds}s pause</span>`:''}${failed?`<span class="tag danger-tag">${failed} failed</span>`:''}</div><div class="progress"><i style="width:${p}%"></i></div><div class="card-head"><span class="tiny">${r.completedSites?.length||0} / ${r.siteIds?.length||0} sites</span><strong>${p}%</strong></div>${controls.length?`<div class="rollout-actions">${controls.join('')}</div>`:''}</article>`}
  function revisionCard(r){return `<article class="revision-card"><div class="card-head"><span class="tag">${r.workloads?.length||0} workloads</span><span class="tiny">${relativeTime(r.createdAt)}</span></div><h3>${escapeHTML(r.name)}</h3><p class="muted tiny">${escapeHTML(r.notes||'No release notes')}</p><div class="cap-list">${(r.workloads||[]).map(w=>`<span class="tag">${escapeHTML(w.kind)} · ${escapeHTML(w.name)}</span>`).join('')}</div></article>`}

  async function renderEvents(){state.events=await api('/api/v1/events?limit=250');const content=document.getElementById('content');content.innerHTML=`<div class="page-head"><div><div class="page-kicker">Audit the edge</div><h1>Events</h1><p>Connectivity, enrollment, rollout and local-agent events remain visible after sites reconnect.</p></div></div><div class="card card-pad">${eventList(state.events)}</div>`}

  async function renderSettings(){
    const admin=state.user.role==='admin';
    const [integrations,tokens,users,apiTokens,webhooks,audit]=await Promise.all([
      api('/api/v1/integrations'),
      admin?api('/api/v1/enrollment-tokens'):Promise.resolve([]),
      admin?api('/api/v1/users'):Promise.resolve([]),
      admin?api('/api/v1/api-tokens'):Promise.resolve([]),
      admin?api('/api/v1/webhooks'):Promise.resolve([]),
      admin?api('/api/v1/audit'):Promise.resolve([])
    ]);
    state.integrations=integrations;state.tokens=tokens;state.users=users;state.apiTokens=apiTokens;state.webhooks=webhooks;state.audit=audit;
    const content=document.getElementById('content');
    content.innerHTML=`<div class="page-head"><div><div class="page-kicker">Control plane</div><h1>Settings</h1><p>Enroll sites, manage operators and connect Fleet to the rest of the Zyvor Platform without coupling the core.</p></div>${admin?'<div class="actions"><button class="ghost-btn" data-action="add-user">Add user</button><button class="primary-btn" data-action="enroll">New enrollment token</button></div>':''}</div>
      <div class="section-title">Zyvor integrations</div><section class="integration-grid">${integrations.map(integrationCard).join('')}</section>
      ${admin?`<div class="section-title">Operators</div><div class="table-card"><table><thead><tr><th>User</th><th>Role</th><th></th></tr></thead><tbody>${users.map(u=>`<tr><td><div class="site-name"><span class="avatar">${escapeHTML(initial(u.name))}</span><div>${escapeHTML(u.name)}<div class="tiny">${escapeHTML(u.email)}</div></div></div></td><td><span class="tag">${escapeHTML(u.role)}</span></td><td>${u.id===state.user.id?'':`<button class="link-btn" data-action="edit-user" data-user="${escapeHTML(u.id)}">Edit</button><button class="link-btn danger-link" data-action="delete-user" data-user="${escapeHTML(u.id)}">Delete</button>`}</td></tr>`).join('')}</tbody></table></div>
      <div class="section-title">Enrollment tokens</div><div class="table-card"><table><thead><tr><th>Name</th><th>Uses</th><th>Created</th><th>Expires</th><th></th></tr></thead><tbody>${tokens.length?tokens.map(t=>`<tr><td>${escapeHTML(t.name)}</td><td>${escapeHTML(t.uses)} / ${escapeHTML(t.maxUses)}</td><td>${relativeTime(t.createdAt)}</td><td>${t.expiresAt?relativeTime(t.expiresAt):'Never'}</td><td><button class="link-btn danger-link" data-action="revoke-token" data-token="${escapeHTML(t.id)}">Revoke</button></td></tr>`).join(''):'<tr><td colspan="5" class="muted tiny">No enrollment tokens yet.</td></tr>'}</tbody></table></div>
      <div class="section-title">Automation API tokens <button class="link-btn" data-action="add-api-token">Create</button></div><div class="table-card"><table><thead><tr><th>Name</th><th>Role / scopes</th><th>Last used</th><th>Expires</th><th></th></tr></thead><tbody>${apiTokens.length?apiTokens.map(t=>`<tr><td>${escapeHTML(t.name)}</td><td><span class="tag">${escapeHTML(t.role)}</span> ${(t.scopes||[]).map(s=>`<span class="tag mono">${escapeHTML(s)}</span>`).join('')}</td><td>${t.lastUsedAt?relativeTime(t.lastUsedAt):'Never'}</td><td>${t.expiresAt?relativeTime(t.expiresAt):'Never'}</td><td><button class="link-btn danger-link" data-action="revoke-api-token" data-token="${escapeHTML(t.id)}">Revoke</button></td></tr>`).join(''):'<tr><td colspan="5" class="muted tiny">No automation tokens yet.</td></tr>'}</tbody></table></div>
      <div class="section-title">Signed webhooks <button class="link-btn" data-action="add-webhook">Add</button></div><div class="table-card"><table><thead><tr><th>Name</th><th>Target</th><th>Filters</th><th>Delivery</th><th></th></tr></thead><tbody>${webhooks.length?webhooks.map(w=>`<tr><td>${escapeHTML(w.name)}</td><td class="mono tiny">${escapeHTML(w.url)}</td><td>${(w.eventKinds||[]).map(k=>`<span class="tag mono">${escapeHTML(k)}</span>`).join('')||'<span class="tag">all events</span>'}</td><td>${w.lastError?`<span class="tag danger-tag">${escapeHTML(w.lastError)}</span>`:(w.lastDeliveryAt?relativeTime(w.lastDeliveryAt):'Waiting')}</td><td><button class="link-btn" data-action="test-webhook" data-webhook="${escapeHTML(w.id)}">Test</button><button class="link-btn danger-link" data-action="delete-webhook" data-webhook="${escapeHTML(w.id)}">Delete</button></td></tr>`).join(''):'<tr><td colspan="5" class="muted tiny">No webhooks configured.</td></tr>'}</tbody></table></div>
      <div class="section-title">Mutation audit</div><div class="table-card"><table><thead><tr><th>Actor</th><th>Request</th><th>Status</th><th>When</th></tr></thead><tbody>${audit.length?audit.slice(0,100).map(a=>`<tr><td>${escapeHTML(a.actor)}</td><td><span class="tag mono">${escapeHTML(a.method)}</span> <span class="mono tiny">${escapeHTML(a.path)}</span></td><td>${escapeHTML(a.status)}</td><td>${relativeTime(a.createdAt)}</td></tr>`).join(''):'<tr><td colspan="4" class="muted tiny">No mutations recorded yet.</td></tr>'}</tbody></table></div>`:''}`;
  }
  function integrationCard(x){
    const fields=x.configuredFields||[];
    return `<article class="integration"><div class="integration-icon">${escapeHTML(initial(x.name))}</div><div><strong>${escapeHTML(x.name)}</strong><p>${escapeHTML(x.purpose)}</p>${fields.length?`<span class="optional">${fields.length} field${fields.length>1?'s':''} configured</span>`:'<span class="optional">Not configured</span>'}</div><div class="integration-actions"><button class="link-btn" data-action="configure-integration" data-integration="${escapeHTML(x.id)}">Configure</button><label class="switch" title="${x.enabled?'Enabled':'Disabled'}"><input type="checkbox" data-action="toggle-integration" data-integration="${escapeHTML(x.id)}" ${x.enabled?'checked':''}><span class="switch-track"></span></label></div></article>`;
  }
  function openAPITokenModal(){const wrap=modal(`<div class="modal-head"><div><h2>Create API token</h2><p>Use scoped credentials for CI and GitOps instead of shared human passwords.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-api-token"><div class="field"><label>Name</label><input name="name" placeholder="github-actions" required></div><div class="field"><label>Role</label><select name="role"><option value="viewer">Viewer</option><option value="operator" selected>Operator</option><option value="admin">Admin</option></select></div><div class="field"><label>Scopes</label><input name="scopes" value="read,rollouts:write" placeholder="read,sites:write,rollouts:write"></div><div class="field"><label>Expires after (hours, 0 = never)</label><input name="expiresHours" type="number" min="0" max="8760" value="720"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create token</button></div></form>`);document.body.appendChild(wrap)}
  function openWebhookModal(){const wrap=modal(`<div class="modal-head"><div><h2>Add signed webhook</h2><p>Deliver Fleet events with HMAC-SHA256 signatures and a durable event cursor.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-webhook"><div class="field"><label>Name</label><input name="name" placeholder="operations" required></div><div class="field"><label>URL</label><input name="url" type="url" placeholder="https://ops.example.net/fleet/events" required></div><div class="field"><label>Event filters</label><input name="eventKinds" value="rollout.*,site.*" placeholder="rollout.*,site.* (blank = all)"></div><div class="field"><label>Signing secret (optional)</label><input name="secret" type="password" autocomplete="new-password" placeholder="Fleet can generate one"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Add webhook</button></div></form>`);document.body.appendChild(wrap)}
  function openIntegrationModal(id){
    const it=(state.integrations||[]).find(x=>x.id===id);if(!it)return;
    const fields=it.configuredFields||[];
    const wrap=modal(`<div class="modal-head"><div><h2>${escapeHTML(it.name)}</h2><p>${escapeHTML(it.purpose)}</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-integration" data-integration="${escapeHTML(it.id)}"><div class="field"><label>API key</label><input name="apiKey" type="password" autocomplete="off" placeholder="${fields.includes('apiKey')?'•••••••• (already set — leave blank to keep)':'Not set'}"></div><div class="field"><label>Base URL</label><input name="url" type="text" autocomplete="off" placeholder="${fields.includes('url')?'Already set — leave blank to keep':'https://…'}"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Save</button></div></form>`);
    document.body.appendChild(wrap);
  }
  function openUserEditModal(id){
    const u=(state.users||[]).find(x=>x.id===id);if(!u)return;
    const wrap=modal(`<div class="modal-head"><div><h2>Edit operator</h2><p>Update role or reset the password for ${escapeHTML(u.email)}.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-user-edit" data-user="${escapeHTML(u.id)}"><div class="field"><label>Name</label><input name="name" value="${escapeHTML(u.name)}" required></div><div class="field"><label>Role</label><select name="role"><option value="viewer" ${u.role==='viewer'?'selected':''}>Viewer</option><option value="operator" ${u.role==='operator'?'selected':''}>Operator</option><option value="admin" ${u.role==='admin'?'selected':''}>Admin</option></select></div><div class="field"><label>Reset password (optional)</label><input name="password" type="password" minlength="10" autocomplete="new-password" placeholder="Leave blank to keep current password"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Save</button></div></form>`);
    document.body.appendChild(wrap);
  }

  function openSite(id){const s=state.sites.find(x=>x.id===id);if(!s)return;const labels=Object.entries(s.labels||{}).map(([k,v])=>`<span class="tag mono">${escapeHTML(k)}=${escapeHTML(v)}</span>`).join('')||'<span class="muted tiny">No labels</span>';const health=(s.workloadHealth||[]).map(h=>`<div class="health-row"><i class="event-bullet ${h.healthy?'success':'error'}"></i><div><strong>${escapeHTML(h.name)}</strong><small>${escapeHTML(h.kind)} · ${h.healthy?'healthy':escapeHTML(h.message||'failed')}</small></div><span class="tiny">${relativeTime(h.checkedAt)}</span></div>`).join('')||'<span class="muted tiny">Health results appear after a revision is reconciled.</span>';const wrap=document.createElement('div');wrap.id='drawer-root';wrap.innerHTML=`<div class="drawer-backdrop" data-action="close-drawer"></div><aside class="drawer"><div class="drawer-top"><button class="ghost-btn" data-action="edit-site" data-site-id="${escapeHTML(s.id)}">Edit metadata</button>${state.user.role==='admin'?`<button class="danger-btn" data-action="delete-site" data-site-id="${escapeHTML(s.id)}">Delete site</button>`:''}<button class="close" data-action="close-drawer" aria-label="Close">×</button></div><div class="eyebrow">${escapeHTML(s.region||'Edge site')}</div><h2>${escapeHTML(s.name)}</h2><span class="status ${statusClass(s.status)}"><i></i>${escapeHTML(s.status)}</span>${s.autonomyMode?'<span class="tag">local autonomy active</span>':''}${s.maintenance?`<span class="tag danger-tag">maintenance${s.maintenanceReason?' · '+escapeHTML(s.maintenanceReason):''}</span>`:''}${s.failedRevision?'<span class="tag danger-tag">revision failed</span>':''}<div class="detail-grid"><div class="detail"><small>Architecture</small><strong>${escapeHTML(s.inventory?.os||'—')} / ${escapeHTML(s.inventory?.arch||'—')}</strong></div><div class="detail"><small>Compute</small><strong>${escapeHTML(s.inventory?.cpuCount||'—')} CPU · ${memory(s.inventory?.memoryBytes)}</strong></div><div class="detail"><small>Agent</small><strong>${escapeHTML(s.agentVersion||'—')}</strong></div><div class="detail"><small>Last seen</small><strong>${relativeTime(s.lastSeen)}</strong></div><div class="detail"><small>Desired revision</small><strong class="mono">${escapeHTML(shortID(s.desiredRevision)||'—')}</strong></div><div class="detail"><small>Applied revision</small><strong class="mono">${escapeHTML(shortID(s.appliedRevision)||'—')}</strong></div></div>${s.revisionError?`<div class="failure-callout"><strong>Last rollout failure</strong><span>${escapeHTML(s.revisionError)}</span></div>`:''}<div class="section-title">Labels & targeting</div><div class="cap-list">${labels}</div><div class="section-title">Workload health</div><div class="health-list">${health}</div><div class="section-title">Detected runtimes</div><div class="cap-list">${(s.inventory?.runtimes||[]).map(x=>`<span class="tag">${escapeHTML(x)}</span>`).join('')||'<span class="muted tiny">None detected</span>'}</div><div class="section-title">Capabilities</div><div class="cap-list">${(s.inventory?.capabilities||[]).map(x=>`<span class="tag">${escapeHTML(x)}</span>`).join('')||'<span class="muted tiny">Standard Linux</span>'}</div><div class="section-title">Addresses</div><div class="mono muted">${(s.inventory?.addresses||[]).map(escapeHTML).join('<br>')||'—'}</div><div class="section-title">Diagnostics</div>${['admin','operator'].includes(state.user.role)?`<div class="actions" style="margin-bottom:14px"><button class="ghost-btn" data-action="run-command" data-site-id="${escapeHTML(s.id)}" data-command-type="agent.ping">Ping agent</button><button class="ghost-btn" data-action="run-command" data-site-id="${escapeHTML(s.id)}" data-command-type="inventory.refresh">Refresh inventory</button></div>`:''}<div id="site-commands-list"><span class="muted tiny">Loading…</span></div></aside>`;document.body.appendChild(wrap);loadSiteCommands(s.id)}
  async function loadSiteCommands(siteId){const el=document.getElementById('site-commands-list');if(!el)return;try{const cmds=await api(`/api/v1/sites/${encodeURIComponent(siteId)}/commands`);if(!document.getElementById('site-commands-list'))return;el.innerHTML=commandsList(cmds)}catch(err){if(el)el.innerHTML=`<span class="muted tiny">Could not load recent commands.</span>`}}
  function commandsList(cmds){if(!cmds.length)return '<span class="muted tiny">No commands sent yet.</span>';return `<div class="event-list">${cmds.map(c=>`<div class="event-row"><i class="event-bullet ${c.status==='completed'?'success':c.status==='failed'?'error':'warning'}"></i><div class="event-text"><strong>${escapeHTML(c.type)}</strong><small>${escapeHTML(c.status)}${c.error?' · '+escapeHTML(c.error):''}</small></div><span class="event-time">${relativeTime(c.updatedAt||c.createdAt)}</span></div>`).join('')}</div>`}

  async function openRevisionModal(){const wrap=modal(`<div class="modal-head"><div><h2>Create revision</h2><p>Declare typed desired state and optional HTTP/TCP health gates. Fleet never sends arbitrary shell.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-revision"><div class="field"><label>Name</label><input name="name" placeholder="Retail stack 2026.09" required></div><div class="field"><label>Release notes</label><input name="notes" placeholder="Promote local API and policy bundle"></div><div class="field"><label>Workloads (JSON)</label><textarea name="workloads" spellcheck="false">[
  {"kind":"systemd","name":"chronyd","state":"running"},
  {"kind":"container","name":"edge-api","state":"running","image":"ghcr.io/example/edge-api:1.4.0","ports":["8081:8080"],"health":{"type":"http","url":"http://127.0.0.1:8081/healthz","expectedStatus":200,"timeoutSeconds":5,"graceSeconds":20}}
]</textarea></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create revision</button></div></form>`);document.body.appendChild(wrap)}

  async function openRolloutModal(){if(!state.revisions.length||!state.sites.length||!state.groups.length){[state.revisions,state.sites,state.groups]=await Promise.all([api('/api/v1/revisions'),api('/api/v1/sites'),api('/api/v1/site-groups')])}if(!state.revisions.length){toast('Create a revision before starting a rollout','error');openRevisionModal();return}if(!state.sites.length){toast('Enroll at least one site before starting a rollout','error');return}const groups=state.groups.map(g=>`<option value="${escapeHTML(g.id)}">${escapeHTML(g.name)} · ${(g.resolvedSiteIds||[]).length} sites</option>`).join('');const wrap=modal(`<div class="modal-head"><div><h2>Start rollout</h2><p>Promote a revision in strict waves with explicit failure and rollback policy.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-rollout"><div class="field"><label>Name</label><input name="name" placeholder="September edge rollout"></div><div class="field"><label>Revision</label><select name="revisionId">${state.revisions.slice().reverse().map(r=>`<option value="${escapeHTML(r.id)}">${escapeHTML(r.name)}</option>`).join('')}</select></div><div class="form-grid"><div class="field"><label>Wave size</label><input name="waveSize" type="number" min="1" max="10000" value="10"></div><div class="field"><label>Pause between waves</label><input name="pauseSeconds" type="number" min="0" max="86400" value="30"></div><div class="field"><label>Maximum failures</label><input name="maxFailures" type="number" min="0" value="0"></div><div class="field"><label>Start at (optional)</label><input name="startAt" type="datetime-local"></div><div class="field"><label>End by (optional)</label><input name="endAt" type="datetime-local"></div></div><div class="field"><label>Dynamic group (optional)</label><select name="groupId"><option value="">Use checked sites below</option>${groups}</select></div><div class="field"><label>Sites</label><div class="check-grid">${state.sites.map(s=>`<label class="check"><input type="checkbox" name="sites" value="${escapeHTML(s.id)}" checked> ${escapeHTML(s.name)} <span class="tiny">${escapeHTML(s.status)}</span></label>`).join('')}</div></div><div class="safety-grid"><label class="check"><input type="checkbox" name="autoRollback"> Auto rollback if failure budget is exceeded</label><label class="check"><input type="checkbox" name="approvalRequired"> Require admin approval before activation</label><label class="check"><input type="checkbox" name="includeMaintenance"> Include maintenance-cordoned sites (break glass)</label></div><div id="rollout-plan-preview" class="plan-preview">Preview the target plan before activation.</div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="preview-rollout">Preview plan</button><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create rollout</button></div></form>`);document.body.appendChild(wrap)}

  function openGroupModal(){const wrap=modal(`<div class="modal-head"><div><h2>Create site group</h2><p>Selectors are evaluated dynamically against current site labels.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-group"><div class="field"><label>Name</label><input name="name" placeholder="Production factories" required></div><div class="field"><label>Description</label><input name="description" placeholder="All production factory sites"></div><div class="field"><label>Label selector</label><input name="selector" placeholder="env=production,class=factory" required></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create group</button></div></form>`);document.body.appendChild(wrap)}

  function openSiteEditModal(id){const s=state.sites.find(x=>x.id===id);if(!s)return;closeDrawer();const labels=Object.entries(s.labels||{}).map(([k,v])=>`${k}=${v}`).join(',');const wrap=modal(`<div class="modal-head"><div><h2>Edit site</h2><p>Labels drive targeting. Maintenance cordons keep a serviced site out of new rollouts without stopping its local desired state.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-site-edit"><input type="hidden" name="id" value="${escapeHTML(s.id)}"><div class="field"><label>Name</label><input name="name" value="${escapeHTML(s.name)}" required></div><div class="field"><label>Region</label><input name="region" value="${escapeHTML(s.region||'')}"></div><div class="field"><label>Labels</label><input name="labels" value="${escapeHTML(labels)}" placeholder="env=prod,zone=west"></div><label class="check"><input type="checkbox" name="maintenance" ${s.maintenance?'checked':''}> Maintenance / rollout cordon</label><div class="field"><label>Maintenance reason</label><input name="maintenanceReason" maxlength="240" value="${escapeHTML(s.maintenanceReason||'')}" placeholder="Scheduled hardware service"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Save site</button></div></form>`);document.body.appendChild(wrap)}

  function openUserModal(){const wrap=modal(`<div class="modal-head"><div><h2>Add operator</h2><p>Create a Fleet account with the minimum role it needs.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-user"><div class="field"><label>Name</label><input name="name" placeholder="Operations Engineer" required></div><div class="field"><label>Email</label><input name="email" type="email" placeholder="ops@example.com" required></div><div class="field"><label>Role</label><select name="role"><option value="viewer">Viewer</option><option value="operator" selected>Operator</option><option value="admin">Admin</option></select></div><div class="field"><label>Temporary password</label><input name="password" type="password" minlength="10" autocomplete="new-password" required></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create user</button></div></form>`);document.body.appendChild(wrap)}

  function openEnrollModal(){const wrap=modal(`<div class="modal-head"><div><h2>Enroll a site</h2><p>Create a short-lived token. The plaintext is displayed once.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><form id="form-enroll"><div class="field"><label>Token name</label><input name="name" value="New edge site" required></div><div class="field"><label>Expires after</label><select name="expiresHours"><option value="1">1 hour</option><option value="24" selected>24 hours</option><option value="168">7 days</option></select></div><div class="field"><label>Maximum uses</label><input name="maxUses" type="number" min="1" max="10000" value="1"></div><div class="modal-actions"><button type="button" class="ghost-btn" data-action="close-modal">Cancel</button><button class="primary-btn" type="submit">Create token</button></div></form>`);document.body.appendChild(wrap)}
  function modal(html){const wrap=document.createElement('div');wrap.id='modal-root';wrap.className='modal-backdrop';wrap.innerHTML=`<section class="modal">${html}</section>`;return wrap}
  function closeModal(){document.getElementById('modal-root')?.remove()}
  function closeDrawer(){document.getElementById('drawer-root')?.remove()}
  function shortID(id){if(!id)return '';return id.length>16?id.slice(0,16)+'…':id}
  function parseLabelPairs(value){const out={};String(value||'').split(',').map(x=>x.trim()).filter(Boolean).forEach(pair=>{const i=pair.indexOf('=');if(i>0)out[pair.slice(0,i).trim()]=pair.slice(i+1).trim()});return out}
  function rolloutPayload(form){const fd=new FormData(form),groupId=fd.get('groupId'),startAt=fd.get('startAt'),endAt=fd.get('endAt');return {name:fd.get('name'),revisionId:fd.get('revisionId'),strategy:'waves',waveSize:Number(fd.get('waveSize')),pauseSeconds:Number(fd.get('pauseSeconds')),maxFailures:Number(fd.get('maxFailures')),siteIds:groupId?[]:fd.getAll('sites'),groupId,autoRollback:fd.get('autoRollback')==='on',approvalRequired:fd.get('approvalRequired')==='on',includeMaintenance:fd.get('includeMaintenance')==='on',startAt:startAt?new Date(startAt).toISOString():null,endAt:endAt?new Date(endAt).toISOString():null}}

  document.addEventListener('click', async e => {
    const route=e.target.closest('[data-route]')?.dataset.route;if(route){document.getElementById('sidebar')?.classList.remove('open');document.getElementById('sidebar-backdrop')?.classList.remove('open');await navigate(route);return}
    const site=e.target.closest('[data-site]')?.dataset.site;if(site){openSite(site);return}
    const action=e.target.closest('[data-action]')?.dataset.action;if(!action)return;
    if(action==='menu'){document.getElementById('sidebar')?.classList.toggle('open');document.getElementById('sidebar-backdrop')?.classList.toggle('open')}
    if(action==='toggle-theme'){toggleTheme();return}
    if(action==='login-back'){loginState.step='identify';renderLoginStep();return}
    if(action==='toggle-password'){const input=document.getElementById('password'),btn=e.target.closest('[data-action="toggle-password"]');if(!input||!btn)return;const showing=input.type==='text';input.type=showing?'password':'text';btn.innerHTML=showing?icons.eye:icons.eyeOff;btn.setAttribute('aria-label',showing?'Show password':'Hide password');return}
    if(action==='toggle-sidebar'){const collapsed=document.querySelector('.shell')?.classList.toggle('collapsed');localStorage.setItem(SIDEBAR_KEY,collapsed?'1':'0');return}
    if(action==='refresh'){await navigate(state.route);toast('Fleet refreshed')}
    if(action==='logout'){try{await api('/api/v1/auth/logout',{method:'POST'})}catch{}state.user=null;loginView()}
    if(action==='close-modal')closeModal();if(action==='close-drawer')closeDrawer();if(action==='new-revision')openRevisionModal();if(action==='new-rollout')await openRolloutModal();if(action==='new-group')openGroupModal();if(action==='enroll')openEnrollModal();if(action==='add-user')openUserModal();if(action==='add-api-token')openAPITokenModal();if(action==='add-webhook')openWebhookModal();
    if(action==='preview-rollout'){const form=document.getElementById('form-rollout');if(!form)return;try{const plan=await api('/api/v1/rollouts/plan',{method:'POST',body:rolloutPayload(form)});const el=document.getElementById('rollout-plan-preview');if(el)el.innerHTML=`<strong>${plan.total} sites · ${plan.waves} waves</strong><span>${plan.offline||0} currently offline · ${(plan.maintenanceExcluded||[]).length} maintenance excluded · wave size ${plan.waveSize}</span>`}catch(err){toast(err.message,'error')}return}
    if(action==='edit-site'){openSiteEditModal(e.target.closest('[data-site-id]')?.dataset.siteId);return}
    if(action==='delete-site'){const id=e.target.closest('[data-site-id]')?.dataset.siteId;const site=state.sites.find(x=>x.id===id);if(!id||!confirm(`Remove ${site?.name||'this site'} from the fleet? This cannot be undone.`))return;try{await api(`/api/v1/sites/${encodeURIComponent(id)}`,{method:'DELETE'});closeDrawer();toast('Site removed');await navigate('sites')}catch(err){toast(err.message,'error')}return}
    if(action==='run-command'){const btn=e.target.closest('[data-action="run-command"]'),id=btn?.dataset.siteId,type=btn?.dataset.commandType;if(!id||!type)return;btn.disabled=true;try{await api(`/api/v1/sites/${encodeURIComponent(id)}/commands`,{method:'POST',body:{type}});toast('Command queued — the agent will pick it up on its next sync');loadSiteCommands(id)}catch(err){toast(err.message,'error')}finally{btn.disabled=false}return}
    if(action==='delete-group'){const id=e.target.closest('[data-group]')?.dataset.group;if(!id||!confirm('Remove this dynamic group? Rollouts already using it are unaffected.'))return;try{await api(`/api/v1/site-groups/${encodeURIComponent(id)}`,{method:'DELETE'});toast('Group removed');await navigate('sites')}catch(err){toast(err.message,'error')}return}
    if(action==='configure-integration'){openIntegrationModal(e.target.closest('[data-integration]')?.dataset.integration);return}
    if(action==='edit-user'){openUserEditModal(e.target.closest('[data-user]')?.dataset.user);return}
    if(action==='delete-user'){const id=e.target.closest('[data-user]')?.dataset.user;if(!id||!confirm('Remove this operator? They will lose access immediately.'))return;try{await api(`/api/v1/users/${encodeURIComponent(id)}`,{method:'DELETE'});toast('User removed');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(action==='revoke-token'){const id=e.target.closest('[data-token]')?.dataset.token;if(!id||!confirm('Revoke this enrollment token? Any pending agent using it will no longer be able to enroll.'))return;try{await api(`/api/v1/enrollment-tokens/${encodeURIComponent(id)}`,{method:'DELETE'});toast('Token revoked');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(action==='revoke-api-token'){const id=e.target.closest('[data-token]')?.dataset.token;if(!id||!confirm('Revoke this automation token?'))return;try{await api(`/api/v1/api-tokens/${encodeURIComponent(id)}`,{method:'DELETE'});toast('API token revoked');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(action==='test-webhook'){const id=e.target.closest('[data-webhook]')?.dataset.webhook;if(!id)return;try{await api(`/api/v1/webhooks/${encodeURIComponent(id)}/test`,{method:'POST'});toast('Webhook test delivered')}catch(err){toast(err.message,'error')}return}
    if(action==='delete-webhook'){const id=e.target.closest('[data-webhook]')?.dataset.webhook;if(!id||!confirm('Delete this webhook?'))return;try{await api(`/api/v1/webhooks/${encodeURIComponent(id)}`,{method:'DELETE'});toast('Webhook deleted');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(action.startsWith('rollout-')){const id=e.target.closest('[data-rollout]')?.dataset.rollout;if(!id)return;const op=action.slice('rollout-'.length);try{await api(`/api/v1/rollouts/${encodeURIComponent(id)}/${op}`,{method:'POST'});toast(`Rollout ${op} complete`);await navigate('rollouts')}catch(err){toast(err.message,'error')}return}
  });

  document.addEventListener('change', async e => {
    const el=e.target.closest('[data-action="toggle-integration"]');if(!el)return;
    const id=el.dataset.integration,enabled=el.checked;
    try{await api(`/api/v1/integrations/${encodeURIComponent(id)}`,{method:'PATCH',body:{enabled}});const it=(state.integrations||[]).find(x=>x.id===id);if(it)it.enabled=enabled;toast(`${it?.name||'Integration'} ${enabled?'enabled':'disabled'}`)}catch(err){el.checked=!enabled;toast(err.message,'error')}
  });

  document.addEventListener('submit', async e => {
    if(e.target.id==='login-form-identify'){e.preventDefault();const fd=new FormData(e.target),email=String(fd.get('email')||'').trim();if(!email)return;loginState.email=email;loginState.step='password';renderLoginStep();return}
    if(e.target.id==='login-form'){e.preventDefault();const fd=new FormData(e.target),btn=e.target.querySelector('button[type=submit]'),errEl=document.getElementById('login-error'),remember=document.getElementById('remember-me')?.checked;btn.disabled=true;errEl.textContent='';try{state.user=await api('/api/v1/auth/login',{method:'POST',body:{email:loginState.email,password:fd.get('password')}});if(remember)localStorage.setItem(LOGIN_SAVE_KEY,loginState.email);else localStorage.removeItem(LOGIN_SAVE_KEY);shell();await navigate(location.hash.slice(1)||'overview')}catch(err){errEl.textContent=err.message}finally{btn.disabled=false}return}
    if(e.target.id==='form-revision'){e.preventDefault();const fd=new FormData(e.target);let workloads;try{workloads=JSON.parse(fd.get('workloads'))}catch{toast('Workloads must be valid JSON','error');return}try{const rev=await api('/api/v1/revisions',{method:'POST',body:{name:fd.get('name'),notes:fd.get('notes'),workloads}});state.revisions.push(rev);closeModal();toast('Revision created');await navigate('rollouts')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-rollout'){e.preventDefault();try{await api('/api/v1/rollouts',{method:'POST',body:rolloutPayload(e.target)});closeModal();toast('Rollout created');await navigate('rollouts')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-group'){e.preventDefault();const fd=new FormData(e.target);try{await api('/api/v1/site-groups',{method:'POST',body:{name:fd.get('name'),description:fd.get('description'),selector:parseLabelPairs(fd.get('selector'))}});closeModal();toast('Site group created');await navigate('sites')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-site-edit'){e.preventDefault();const fd=new FormData(e.target);try{await api(`/api/v1/sites/${encodeURIComponent(fd.get('id'))}`,{method:'PATCH',body:{name:fd.get('name'),region:fd.get('region'),labels:parseLabelPairs(fd.get('labels')),maintenance:fd.get('maintenance')==='on',maintenanceReason:fd.get('maintenanceReason')}});closeModal();toast('Site settings updated');await navigate('sites')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-api-token'){e.preventDefault();const fd=new FormData(e.target);const scopes=String(fd.get('scopes')||'').split(',').map(x=>x.trim()).filter(Boolean);try{const result=await api('/api/v1/api-tokens',{method:'POST',body:{name:fd.get('name'),role:fd.get('role'),scopes,expiresHours:Number(fd.get('expiresHours'))}});document.querySelector('#modal-root .modal').innerHTML=`<div class="modal-head"><div><h2>API token</h2><p>Copy it now. Fleet stores only its SHA-256 digest.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><div class="token-box">${escapeHTML(result.token)}</div><div class="field"><label>Environment</label><div class="token-box">ZYVOR_FLEET_API_TOKEN='${escapeHTML(result.token)}'</div></div><div class="modal-actions"><button class="primary-btn" type="button" data-action="close-modal">Done</button></div>`}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-webhook'){e.preventDefault();const fd=new FormData(e.target);const eventKinds=String(fd.get('eventKinds')||'').split(',').map(x=>x.trim()).filter(Boolean);try{const result=await api('/api/v1/webhooks',{method:'POST',body:{name:fd.get('name'),url:fd.get('url'),secret:fd.get('secret'),eventKinds}});document.querySelector('#modal-root .modal').innerHTML=`<div class="modal-head"><div><h2>Webhook ready</h2><p>Store this signing secret securely; it is shown once when Fleet generated or accepted it.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><div class="token-box">${escapeHTML(result.signingSecret)}</div><div class="modal-actions"><button class="primary-btn" type="button" data-action="close-modal">Done</button></div>`}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-user'){e.preventDefault();const fd=new FormData(e.target);try{await api('/api/v1/users',{method:'POST',body:{name:fd.get('name'),email:fd.get('email'),role:fd.get('role'),password:fd.get('password')}});closeModal();toast('User created');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-user-edit'){e.preventDefault();const fd=new FormData(e.target),id=e.target.dataset.user;const body={name:fd.get('name'),role:fd.get('role')};const password=fd.get('password');if(password)body.password=password;try{await api(`/api/v1/users/${encodeURIComponent(id)}`,{method:'PATCH',body});closeModal();toast('User updated');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-integration'){e.preventDefault();const fd=new FormData(e.target),id=e.target.dataset.integration;const config={};for(const key of ['apiKey','url']){const v=fd.get(key);if(v)config[key]=v}try{await api(`/api/v1/integrations/${encodeURIComponent(id)}`,{method:'PATCH',body:{config}});closeModal();toast('Integration updated');await navigate('settings')}catch(err){toast(err.message,'error')}return}
    if(e.target.id==='form-enroll'){e.preventDefault();const fd=new FormData(e.target);try{const result=await api('/api/v1/enrollment-tokens',{method:'POST',body:{name:fd.get('name'),expiresHours:Number(fd.get('expiresHours')),maxUses:Number(fd.get('maxUses'))}});document.querySelector('#modal-root .modal').innerHTML=`<div class="modal-head"><div><h2>Enrollment token</h2><p>Copy it now. Fleet stores only its SHA-256 digest.</p></div><button class="close" data-action="close-modal" aria-label="Close">×</button></div><div class="token-box">${escapeHTML(result.token)}</div><div class="field"><label>Start an agent</label><div class="token-box">ZYVOR_FLEET_ENROLLMENT_TOKEN='${escapeHTML(result.token)}' fleet-agent --server ${escapeHTML(location.origin)} --name edge-site-01</div></div><div class="modal-actions"><button class="primary-btn" type="button" data-action="close-modal">Done</button></div>`}catch(err){toast(err.message,'error')}return}
  });

  window.addEventListener('hashchange',()=>{const route=location.hash.slice(1);if(state.user&&route&&route!==state.route)navigate(route)});

  async function boot(){app.innerHTML='<div class="app-loading"><div class="spinner"></div></div>';try{state.user=await api('/api/v1/auth/me');state.route=location.hash.slice(1)||'overview';shell();await navigate(state.route)}catch{loginView()}}
  boot();
})();
