let currentDatabase = null;
let currentTable = null;

// Load initial data
async function loadStatus() {
    try {
        const res = await fetch('/status');
        const data = await res.json();
        if (data.success) {
            renderDatabases(data.data.databases);
            renderWorkerStatus(data.data.workers);
        }
    } catch (err) {
        console.error('Failed to load status:', err);
    }
}

function renderDatabases(databases) {
    const container = document.getElementById('databasesList');
    if (!databases || databases.length === 0) {
        container.innerHTML = '<p class="text-gray-500">No databases</p>';
        return;
    }
    container.innerHTML = databases.map(db => `
        <div class="flex justify-between items-center p-2 bg-gray-50 rounded cursor-pointer hover:bg-gray-100"
             onclick="selectDatabase('${db}')">
            <span>📁 ${db}</span>
            <button onclick="event.stopPropagation(); deleteDatabase('${db}')" class="text-red-500 hover:text-red-700">🗑️</button>
        </div>
    `).join('');
}

function selectDatabase(dbName) {
    currentDatabase = dbName;
    loadTables(dbName);
}

async function loadTables(dbName) {
    try {
        const res = await fetch('/status');
        const data = await res.json();
        // Parse tables from databases structure (simplified)
        document.getElementById('tablesList').innerHTML = '<div class="text-gray-500">Loading tables...</div>';
        // For simplicity, call a tables endpoint or parse from status
        // In production, add a /tables endpoint
    } catch (err) {
        console.error(err);
    }
}

function renderWorkerStatus(workers) {
    const container = document.getElementById('workerStatus');
    if (!workers) return;
    container.innerHTML = workers.map(w => `
        <span class="px-3 py-1 rounded-full ${w.alive ? 'bg-green-200 text-green-800' : 'bg-red-200 text-red-800'}">
            ${w.address.split(':')[1] ? w.address.split(':')[1] : w.address} 
            ${w.alive ? '🟢' : '🔴'}
        </span>
    `).join('');
}

async function createDatabase() {
    const name = prompt('Database name:');
    if (!name) return;
    const res = await fetch('/create-db', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
    });
    if (res.ok) {
        alert(`Database "${name}" created`);
        loadStatus();
    } else {
        alert('Failed to create database');
    }
}

async function deleteDatabase(name) {
    if (!confirm(`Delete database "${name}"? All data will be lost.`)) return;
    const res = await fetch('/drop-db', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name })
    });
    if (res.ok) {
        alert(`Database "${name}" deleted`);
        loadStatus();
    } else {
        alert('Failed to delete database');
    }
}

async function executeQuery() {
    const rawQuery = document.getElementById('queryInput').value;
    const resultDiv = document.getElementById('queryResult');
    try {
        let query;
        if (rawQuery.trim().startsWith('{')) {
            query = JSON.parse(rawQuery);
        } else {
            // Simple SQL-like parsing for demo
            alert('Use JSON format for now');
            return;
        }
        
        let url = '';
        let options = { method: 'GET' };
        
        switch (query.operation) {
            case 'select':
                url = `/select?database=${query.database}&table=${query.table}`;
                break;
            case 'insert':
                url = '/insert';
                options = { method: 'POST', body: JSON.stringify(query) };
                break;
            default:
                alert('Operation not supported in query editor yet');
                return;
        }
        
        options.headers = { 'Content-Type': 'application/json' };
        const res = await fetch(url, options);
        const data = await res.json();
        resultDiv.classList.remove('hidden');
        resultDiv.textContent = JSON.stringify(data, null, 2);
    } catch (err) {
        resultDiv.classList.remove('hidden');
        resultDiv.textContent = 'Error: ' + err.message;
    }
}

// Initial load
loadStatus();
setInterval(loadStatus, 5000); // Refresh every 5 seconds