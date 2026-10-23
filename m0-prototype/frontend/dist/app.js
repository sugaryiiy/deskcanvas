// DeskCanvas M0 frontend: poll Go state, reflect mode, wire the panel.
// Communication is plain fetch() to the /api/* handler in app.go — no
// generated Wails bindings needed for the prototype.

const badge = document.getElementById('mode-badge');
const toggleBtn = document.getElementById('toggle');
let mode = 'locked';

async function refresh() {
  try {
    const s = await (await fetch('/api/state')).json();
    setModeUI(s.mode);
    document.querySelectorAll('#offsets button').forEach(b =>
      b.classList.toggle('active', Number(b.dataset.off) === s.levelOffset));
  } catch (e) { /* app still starting */ }
}

function setModeUI(m) {
  mode = m;
  document.body.classList.toggle('edit', m === 'edit');
  document.body.classList.toggle('locked', m === 'locked');
  badge.textContent = m === 'edit' ? '编辑模式 EDIT' : '锁定模式 LOCKED';
  badge.className = 'badge ' + m;
  toggleBtn.textContent = m === 'edit' ? '沉回桌面（锁定）' : '切换到编辑模式';
}

toggleBtn.addEventListener('click', async () => {
  const s = await (await fetch('/api/toggle', { method: 'POST' })).json();
  setModeUI(s.mode);
});

document.getElementById('offsets').addEventListener('click', async (e) => {
  const b = e.target.closest('button');
  if (!b) return;
  await fetch('/api/level', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ offset: Number(b.dataset.off) }),
  });
  refresh();
});

// Card dragging, edit mode only. In locked mode the OS ignores mouse
// events for the whole window, so this code simply never fires there —
// that asymmetry is itself one of the M0 checks.
document.querySelectorAll('.card').forEach(card => {
  const head = card.querySelector('header');
  head.addEventListener('pointerdown', (e) => {
    if (mode !== 'edit') return;
    const r = card.getBoundingClientRect();
    const dx = e.clientX - r.left, dy = e.clientY - r.top;
    card.style.right = 'auto'; card.style.bottom = 'auto';
    function move(ev) {
      card.style.left = (ev.clientX - dx) + 'px';
      card.style.top = (ev.clientY - dy) + 'px';
    }
    function up() {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    }
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  });
});

refresh();
setInterval(refresh, 600);
