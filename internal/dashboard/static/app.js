// Drover Dashboard - Vanilla JavaScript

(function() {
  'use strict';

  // State
  let ws = null;
  let currentView = 'overview';
  let stats = null;
  let epics = [];
  let tasks = [];
  let workers = [];
  let graph = null;
  let activity = [];

  // DOM Elements
  const connectionStatus = document.getElementById('connection-status');
  const progressFill = document.getElementById('progress-fill');
  const progressPercent = document.getElementById('progress-percent');
  const activityLog = document.getElementById('activity-log');

  // Initialize
  function init() {
    setupNavigation();
    setupFilters();
    connectWebSocket();
    loadInitialData();
    setInterval(loadInitialData, 5000); // Poll every 5s as fallback
  }

  // Navigation
  function setupNavigation() {
    const navBtns = document.querySelectorAll('.nav-btn');
    navBtns.forEach(btn => {
      btn.addEventListener('click', () => {
        navBtns.forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        switchView(btn.dataset.view);
      });
    });
  }

  function switchView(view) {
    currentView = view;
    document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
    document.getElementById(`view-${view}`).classList.add('active');
    renderCurrentView();
  }

  // Filters
  function setupFilters() {
    const epicFilter = document.getElementById('filter-epic');
    const statusFilter = document.getElementById('filter-status');

    epicFilter.addEventListener('change', () => loadTasks());
    statusFilter.addEventListener('change', () => loadTasks());
  }

  // WebSocket Connection
  function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      connectionStatus.textContent = 'Live';
      connectionStatus.classList.remove('offline');
      connectionStatus.classList.add('online');
    };

    ws.onclose = () => {
      connectionStatus.textContent = 'Offline';
      connectionStatus.classList.remove('online');
      connectionStatus.classList.add('offline');
      // Reconnect after 3s
      setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = () => {
      connectionStatus.textContent = 'Error';
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleWebSocketMessage(msg);
      } catch (e) {
        console.error('Failed to parse WebSocket message:', e);
      }
    };
  }

  function handleWebSocketMessage(msg) {
    switch (msg.type) {
      case 'stats_update':
        stats = msg.data;
        updateOverview();
        break;
      case 'task_claimed':
        addActivity(`Task claimed: ${msg.data.title}`, 'info');
        loadInitialData();
        break;
      case 'task_started':
        addActivity(`Task started: ${msg.data.title}`, 'info');
        loadInitialData();
        break;
      case 'task_completed':
        addActivity(`Task completed: ${msg.data.title}`, 'success');
        loadInitialData();
        break;
      case 'task_failed':
        addActivity(`Task failed: ${msg.data.title} - ${msg.data.error}`, 'error');
        loadInitialData();
        break;
    }
  }

  // API Calls
  async function api(path) {
    try {
      const res = await fetch(path);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      return await res.json();
    } catch (e) {
      console.error(`API error for ${path}:`, e);
      return null;
    }
  }

  async function loadInitialData() {
    stats = await api('/api/status');
    epics = await api('/api/epics') || [];
    tasks = await api('/api/tasks') || [];
    workers = await api('/api/workers') || [];
    graph = await api('/api/graph');

    updateOverview();
    updateEpicFilter();
    renderCurrentView();
  }

  async function loadTasks() {
    const epic = document.getElementById('filter-epic').value;
    const status = document.getElementById('filter-status').value;

    let path = '/api/tasks?';
    if (epic) path += `epic=${encodeURIComponent(epic)}&`;
    if (status) path += `status=${status}&`;

    tasks = await api(path) || [];
    renderTasks();
  }

  // Rendering
  function updateOverview() {
    if (!stats) return;

    document.getElementById('stat-ready').textContent = stats.ready;
    document.getElementById('stat-active').textContent = stats.claimed + stats.in_progress;
    document.getElementById('stat-blocked').textContent = stats.blocked;
    document.getElementById('stat-done').textContent = stats.completed;
    document.getElementById('stat-failed').textContent = stats.failed;

    progressPercent.textContent = `${stats.progress}%`;
    progressFill.style.width = `${stats.progress}%`;
  }

  function updateEpicFilter() {
    const select = document.getElementById('filter-epic');
    const currentValue = select.value;
    select.innerHTML = '<option value="">All Epics</option>';
    epics.forEach(epic => {
      const opt = document.createElement('option');
      opt.value = epic.id;
      opt.textContent = epic.title;
      select.appendChild(opt);
    });
    select.value = currentValue;
  }

  function renderCurrentView() {
    switch (currentView) {
      case 'overview':
        // Overview is updated by updateOverview()
        break;
      case 'epics':
        renderEpics();
        break;
      case 'tasks':
        renderTasks();
        break;
      case 'workers':
        renderWorkers();
        break;
      case 'graph':
        renderGraph();
        break;
    }
  }

  function renderEpics() {
    const container = document.getElementById('epics-list');
    if (!epics.length) {
      container.innerHTML = '<div class="empty-state">No epics yet</div>';
      return;
    }

    container.innerHTML = epics.map(epic => `
      <div class="epic-card">
        <div class="epic-header">
          <h3>${escapeHtml(epic.title)}</h3>
          <span class="badge ${epic.status}">${epic.status}</span>
        </div>
        <p class="epic-description">${escapeHtml(epic.description || 'No description')}</p>
        <div class="epic-stats">
          <span>${epic.completed}/${epic.task_count} done</span>
          <span>${epic.ready} ready</span>
          <span>${epic.active} active</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" style="width: ${epic.task_count ? (epic.completed * 100 / epic.task_count) : 0}%"></div>
        </div>
      </div>
    `).join('');
  }

  function renderTasks() {
    const container = document.getElementById('tasks-list');
    if (!tasks.length) {
      container.innerHTML = '<div class="empty-state">No tasks found</div>';
      return;
    }

    container.innerHTML = tasks.map(task => `
      <div class="task-card status-${task.status}">
        <div class="task-header">
          <span class="task-id">${escapeHtml(task.id)}</span>
          <span class="badge ${task.status}">${task.status}</span>
        </div>
        <h4>${escapeHtml(task.title)}</h4>
        ${task.description ? `<p class="task-description">${escapeHtml(task.description)}</p>` : ''}
        ${task.epic_title ? `<div class="task-epic">📋 ${escapeHtml(task.epic_title)}</div>` : ''}
        ${task.claimed_by ? `<div class="task-worker">👷 ${escapeHtml(task.claimed_by)}</div>` : ''}
        ${task.last_error ? `<div class="task-error">❌ ${escapeHtml(task.last_error)}</div>` : ''}
      </div>
    `).join('');
  }

  function renderWorkers() {
    const container = document.getElementById('workers-list');
    if (!workers.length) {
      container.innerHTML = '<div class="empty-state">No active workers</div>';
      return;
    }

    container.innerHTML = workers.map(worker => {
      const duration = formatDuration(worker.duration);
      return `
        <div class="worker-card">
          <div class="worker-info">
            <span class="worker-name">${escapeHtml(worker.worker_id)}</span>
            <span class="worker-task">${escapeHtml(worker.title)}</span>
            <span class="worker-id">${escapeHtml(worker.task_id)}</span>
          </div>
          <div class="worker-duration">⏱ ${duration}</div>
        </div>
      `;
    }).join('');
  }

  function renderGraph() {
    const svg = document.getElementById('dependency-graph');
    if (!graph || !graph.nodes.length) {
      svg.innerHTML = '<text x="50%" y="50%" text-anchor="middle">No dependencies to display</text>';
      return;
    }

    // Simple layered layout
    const layers = buildLayers(graph.nodes, graph.edges);
    const nodePositions = positionNodes(layers, graph);

    let edgesSvg = '';
    graph.edges.forEach(edge => {
      const from = nodePositions[edge.from];
      const to = nodePositions[edge.to];
      if (from && to) {
        edgesSvg += `<line x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}" class="edge"/>`;
      }
    });

    let nodesSvg = '';
    graph.nodes.forEach(node => {
      const pos = nodePositions[node.id];
      if (pos) {
        nodesSvg += `
          <g class="node status-${node.status}" transform="translate(${pos.x}, ${pos.y})">
            <rect x="-60" y="-20" width="120" height="40" rx="4"/>
            <text text-anchor="middle" dominant-baseline="middle">${truncate(escapeHtml(node.title), 15)}</text>
          </g>
        `;
      }
    });

    svg.innerHTML = edgesSvg + nodesSvg;
  }

  function buildLayers(nodes, edges) {
    // Build adjacency and in-degree maps
    const adj = new Map();
    const inDegree = new Map();
    nodes.forEach(n => {
      adj.set(n.id, []);
      inDegree.set(n.id, 0);
    });
    edges.forEach(e => {
      adj.get(e.from).push(e.to);
      inDegree.set(e.to, (inDegree.get(e.to) || 0) + 1);
    });

    // Kahn's algorithm for topological sort with layers
    const layers = [];
    const remaining = new Map(inDegree);
    const processed = new Set();

    while (processed.size < nodes.length) {
      const layer = [];
      nodes.forEach(n => {
        if (!processed.has(n.id) && (remaining.get(n.id) || 0) === 0) {
          layer.push(n.id);
        }
      });

      if (layer.length === 0 && processed.size < nodes.length) {
        // Cycle detected - add remaining nodes to current layer
        nodes.forEach(n => {
          if (!processed.has(n.id)) layer.push(n.id);
        });
      }

      layer.forEach(id => {
        processed.add(id);
        adj.get(id).forEach(next => {
          remaining.set(next, Math.max(0, (remaining.get(next) || 1) - 1));
        });
      });

      layers.push(layer);
    }

    return layers;
  }

  function positionNodes(layers, graph) {
    const positions = {};
    const layerHeight = 80;
    const nodeWidth = 140;

    layers.forEach((layer, layerIndex) => {
      const y = layerIndex * layerHeight + 60;
      const totalWidth = layer.length * nodeWidth;
      const startX = (800 - totalWidth) / 2 + nodeWidth / 2;

      layer.forEach((nodeId, i) => {
        positions[nodeId] = {
          x: startX + i * nodeWidth,
          y: y
        };
      });
    });

    return positions;
  }

  // Activity Log
  function addActivity(message, type = 'info') {
    const time = new Date().toLocaleTimeString();
    activity.unshift({ message, type, time });
    if (activity.length > 20) activity.pop();
    renderActivity();
  }

  function renderActivity() {
    activityLog.innerHTML = activity.map(a => `
      <div class="activity-item ${a.type}">
        <span class="activity-time">${a.time}</span>
        <span class="activity-message">${escapeHtml(a.message)}</span>
      </div>
    `).join('');
  }

  // Utilities
  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  function truncate(text, maxLen) {
    return text.length > maxLen ? text.substring(0, maxLen) + '...' : text;
  }

  function formatDuration(seconds) {
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
    return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
  }

  // Start
  document.addEventListener('DOMContentLoaded', init);
})();
