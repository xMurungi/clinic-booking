const $ = (s) => document.querySelector(s);
const DOCTORS = { 1: "Dr. Amina Hassan", 2: "Dr. Brian Otieno", 3: "Dr. Carol Wanjiru", 4: "Dr. David Kimani", 5: "Dr. Esther Njeri" };
const PATIENTS = { 1: "Test Patient One", 2: "Test Patient Two" };
let selected = null; // start time of the slot the user clicked

const fmt = (iso, opts) => new Date(iso).toLocaleString("en-GB", { timeZone: "Africa/Nairobi", ...opts });
const hhmm = (iso) => fmt(iso, { hour: "2-digit", minute: "2-digit" });

function say(text, ok) { const m = $("#msg"); m.textContent = text; m.className = ok ? "ok" : "err"; }

async function api(path, method = "GET", body) {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(`${res.status} ${data.error?.code ?? ""}: ${data.error?.message ?? res.statusText}`);
  return data;
}

async function run(fn) { try { await fn(); } catch (e) { say(e.message, false); } }

async function loadSlots() {
  selected = null;
  const { slots } = await api(`/doctors/${$("#doctor").value}/availability?date=${$("#date").value}`);
  const box = $("#slots");
  box.innerHTML = slots.length ? "" : '<p class="muted">No free slots on this day.</p>';
  for (const s of slots) {
    const b = document.createElement("button");
    b.textContent = hhmm(s.start);
    b.onclick = () => {
      selected = s.start;
      box.querySelectorAll("button").forEach((x) => x.classList.remove("on"));
      b.classList.add("on");
    };
    box.append(b);
  }
}

async function loadAppts() {
  const { appointments } = await api(`/patients/${$("#patient").value}/appointments`);
  const box = $("#appts");
  box.innerHTML = appointments.length ? "" : '<p class="muted">No upcoming appointments.</p>';
  for (const a of appointments) {
    const row = document.createElement("div");
    row.className = "appt";
    row.innerHTML = `<span>#${a.id} · ${DOCTORS[a.doctor_id] ?? "Doctor " + a.doctor_id}<br>
      ${fmt(a.start_time, { weekday: "short", day: "numeric", month: "short" })}, ${hhmm(a.start_time)}</span>
      <div><button data-act="move">Move to selected slot</button><button data-act="cancel">Cancel</button></div>`;
    row.querySelector('[data-act="cancel"]').onclick = () => run(async () => {
      const reason = prompt("Reason for cancelling?");
      if (!reason) return;
      await api(`/appointments/${a.id}/cancel`, "PATCH", { reason });
      say(`Appointment #${a.id} cancelled.`, true);
      await Promise.all([loadAppts(), loadSlots()]);
    });
    row.querySelector('[data-act="move"]').onclick = () => run(async () => {
      if (!selected) throw new Error("Pick a new slot above first.");
      await api(`/appointments/${a.id}/reschedule`, "PATCH", { start_time: selected });
      say(`Appointment #${a.id} moved to ${hhmm(selected)}.`, true);
      await Promise.all([loadAppts(), loadSlots()]);
    });
    box.append(row);
  }
}

$("#book").onclick = () => run(async () => {
  if (!selected) throw new Error("Pick a slot first.");
  const a = await api("/appointments", "POST", {
    doctor_id: +$("#doctor").value, patient_id: +$("#patient").value, start_time: selected,
  });
  say(`Booked appointment #${a.id} at ${hhmm(a.start_time)}.`, true);
  await Promise.all([loadAppts(), loadSlots()]);
});

for (const [id, name] of Object.entries(DOCTORS)) $("#doctor").add(new Option(name, id));
for (const [id, name] of Object.entries(PATIENTS)) $("#patient").add(new Option(name, id));
$("#date").value = new Date(Date.now() + 864e5).toISOString().slice(0, 10);
$("#doctor").onchange = $("#date").onchange = () => run(loadSlots);
$("#patient").onchange = () => run(loadAppts);
run(async () => { await loadSlots(); await loadAppts(); });
