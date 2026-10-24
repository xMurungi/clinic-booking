# Clinic Booking API

A small REST API for booking appointments at a clinic with 5 doctors, written in Go with Postgres.

- **Live URL:** https://clinic-booking-8a1y.onrender.com/healthz (This comes with a demo ui to interact with the backend)
> You must add the api endpoints to the url above for it to work as intended
- **Repo:** <https://github.com/xMurungi/clinic-booking>
- **Deploys from:** the `main` branch (see CI/CD below)

> The app runs on Render's free tier, so the first request after a quiet period can take 30-60 seconds while it wakes up. Hit `/healthz` first and give it a minute.

## Try it

```bash
BASE= https://clinic-booking-8a1y.onrender.com

curl $BASE/healthz
curl "$BASE/doctors/1/availability?date=2026-10-26"

curl -X POST $BASE/appointments -H 'Content-Type: application/json' \
  -d '{"doctor_id":1,"patient_id":1,"start_time":"2026-10-26T10:00:00+03:00"}'
```

Five doctors (ids 1-5) and two patients (ids 1-2) are created by a seed migration. All have Mon-Fri 09:00-17:00 hours, and doctor 1 also works Saturday 09:00-13:00.

---

## Section 1: System design

### What the system has to do

Show a patient which 30-minute slots a doctor has free on a given day, let them book one, make sure nobody else can take it, and let them cancel or move it.

### Models

| Table | What it holds |
|---|---|
| `doctors` | id, name, specialty |
| `working_hours` | one row per doctor per weekday: start and end time |
| `patients` | id, name, phone, email |
| `appointments` | doctor, patient, slot start time, status (`booked` / `cancelled`), cancel reason |

### Components

```
HTTP handlers  ->  booking service  ->  store (Postgres)
(status codes)     (the rules)          (SQL only)
                        |
                  slot logic (pure functions, no DB)
```

- **Handlers** (`internal/handlers`): parse requests, call the service, turn errors into status codes. No business rules.
- **Service** (`internal/booking`): every rule lives here (working hours, past slots, lead time, cancelled checks).
- **Slot logic** (`internal/booking/slots.go`): plain functions that build and filter slots. No database, so they're easy to test.
- **Store** (`internal/store`): all the SQL. The service only sees an interface, so tests can swap in an in-memory fake.

### Key decisions

**1. Slots are calculated, not stored.**
Each request builds the day's slots from the doctor's working hours and removes the booked ones. The alternative was a `slots` table with a row for every slot, which would need a job to keep generating future slots and would need updating whenever hours change. Calculating costs a little CPU per request, which is nothing at this size.

**2. The database stops double booking, not the code.**
A check-then-insert in the code isn't safe, because two requests can both see the slot as free before either inserts. So there's a unique partial index:

```sql
CREATE UNIQUE INDEX one_active_booking_per_slot
  ON appointments (doctor_id, start_time) WHERE status = 'booked';
```

The second insert fails, and the app turns that into a `409`. It also works with several app instances. Because it only covers `booked` rows, a cancelled slot can be booked again without deleting anything.

**3. Cancel and reschedule are single `UPDATE ... WHERE status = 'booked'` statements.**
If two cancels arrive at once, only one matches. Reschedule changes `start_time` on the same row, so the appointment keeps its id, and the same index checks the new slot.

**4. Time handling.**
Times are stored in UTC. Working hours are stored as wall-clock times and read as `Africa/Nairobi`. Requests must send RFC3339 with an offset (`2026-10-26T10:00:00+03:00`), so there's no guessing about timezones. Slot comparisons use the actual instant, so UTC from the database matches Nairobi times from the code.

**5. Slot rules are shared.**
Booking and rescheduling use the same `validateSlot` function, so a reschedule is checked exactly like a fresh booking.

**6. A clock is passed into the service** instead of calling `time.Now()` inside it. That makes the "in the past" and "1 hour ahead" rules testable.

### Trade-offs

- **No auth.** The brief doesn't ask for it, so any caller can cancel any appointment. For real use, patients would log in and could only touch their own bookings.
- **No endpoints to create doctors or patients.** They come from the seed migration. The brief only covers appointments.
- **One working window per doctor per weekday.** That's simple, but it can't model a lunch break. Allowing several windows would mean dropping the `UNIQUE (doctor_id, weekday)` constraint and generating slots per window.
- **Cancelling is allowed at any time.** A real clinic would likely add a cutoff (for example, nothing within 2 hours).
- **Offset-based pagination isn't needed yet.** `GET /patients/{id}/appointments` returns everything upcoming. Fine for now, but it would need paging eventually.

### If the clinic grows

- Doctor leave, holidays and breaks (an exceptions table that's subtracted from working hours).
- Several clinics or branches (a `clinic_id` on doctors and appointments).
- Different slot lengths per doctor or appointment type.
- Auth, plus SMS or email reminders sent from a queue.
- Read replicas or caching for availability if traffic gets high. The unique index would stay on the primary.

---

## Section 2: API

Errors always look like this:

```json
{ "error": { "code": "slot_taken", "message": "slot is already booked" } }
```

### Endpoints

| Method | Path | Success | Description |
|---|---|---|---|
| GET | `/healthz` | 200 | health check |
| GET | `/doctors/{id}/availability?date=YYYY-MM-DD` | 200 | free slots for that doctor on that day |
| POST | `/appointments` | 201 | book a slot |
| PATCH | `/appointments/{id}/cancel` | 200 | cancel with a reason |
| PATCH | `/appointments/{id}/reschedule` | 200 | move to a new slot |
| GET | `/patients/{id}/appointments` | 200 | upcoming appointments, soonest first |

**Book:** `{"doctor_id":1,"patient_id":1,"start_time":"2026-10-26T10:00:00+03:00"}`
**Cancel:** `{"reason":"patient travelling"}`
**Reschedule:** `{"start_time":"2026-10-26T14:00:00+03:00"}`

### Status codes

| Code | When |
|---|---|
| 400 | bad JSON, missing fields, bad id or date, empty cancel reason |
| 404 | doctor, patient or appointment doesn't exist |
| 409 | slot already booked, or appointment is already cancelled |
| 422 | slot isn't on a :00/:30 boundary, outside working hours, in the past or less than 1 hour away, or rescheduling to the same time |
| 500 | anything unexpected (logged, with a request id) |

A day when the doctor doesn't work returns `200` with an empty slot list, because that's a valid answer rather than an error.

### Rules decided where the brief was vague

- A booking must start at least **1 hour from now** (the bonus rule). Availability hides slots inside that window too, so what you see matches what you can book.
- Slots must fit fully inside working hours, so the last slot with 09:00-17:00 hours is 16:30.
- Rescheduling to the time the appointment already has returns `422`.
- Cancelled appointments don't appear in a patient's upcoming list and don't block their slot.

### Project layout

```
cmd/api/            main: wiring, startup, graceful shutdown
internal/config/    env config
internal/domain/    models and error values
internal/booking/   service + slot logic (+ tests)
internal/store/     Postgres queries
internal/handlers/  HTTP handlers (+ tests)
internal/httpx/     JSON helpers, request id, logging, panic recovery
migrations/         SQL schema + seed (embedded into the binary)
```

---

## Run it locally

You need Go 1.23+ and Docker.

```bash
docker compose up -d --wait
export DATABASE_URL='postgres://clinic-booking:clinic-booking@localhost:5432/clinic-booking?sslmode=disable'
go run ./cmd/api
```

Migrations and seed data run automatically on startup. The server listens on `:8080` (or `PORT` / `HTTP_ADDR` if set).

### Tests

```bash
go test ./... -race
```

- **Slot logic:** generation, filtering, grid and working-hours checks, UTC vs Nairobi comparison.
- **Service:** booking rules, cancel, reschedule, lead time, patient appointments, using an in-memory fake store that follows the same rules as the DB.
- **Handlers:** every error maps to the right status code.

**What the tests don't cover:** the SQL. The fake store can't catch a wrong column name or a bad query. I hit this while building (a typo'd column name only failed against real Postgres), and I checked the database behaviour by hand instead. A proper Postgres integration test is the next thing I'd add.

### Double booking check

Ten requests fired at the same slot at the same time:

```bash
for i in $(seq 10); do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST localhost:8080/appointments \
    -H 'Content-Type: application/json' \
    -d '{"doctor_id":3,"patient_id":1,"start_time":"2026-10-28T11:00:00+03:00"}' &
done | sort | uniq -c
```
```
   1 201
   9 409
```

---

## Section 3: Deployment and CI/CD

**Host:** Render (Docker web service + managed Postgres). I picked it because it's quick to set up and gives a managed database, and I wanted my time to go into the API rather than infrastructure. Free-tier limits I know about: the web service sleeps after 15 minutes idle, and the free database expires after 30 days. Both are fine for a take-home but wouldn't be for production.

**Pipeline** (`.github/workflows/ci.yml`):
- **On every pull request:** `go vet` and `go test -race`.
- **On merge (push) to `main`:** the same tests run first, and only if they pass, a job calls Render's deploy hook, which builds the Docker image and rolls it out.
- Render's own auto-deploy is turned off, so a deploy only happens after the tests pass in GitHub.
- The deploy hook URL is stored as the `RENDER_DEPLOY_HOOK` GitHub secret.

**Startup behaviour:** the app waits up to 30 seconds for the database to accept connections, then runs the embedded migrations, then starts serving. This matters because the database can still be starting when the app boots.

---

## Section 4: AI reflection

**1. What I used AI for**
- **Section 1:** talking through the design, especially how to stop two people booking the same slot.
- **Section 2:** writing a first version of the slot logic, services, handlers and tests, which I then ran, fixed and changed.
- **Section 3:** the Dockerfile and the GitHub Actions workflow, and comparing hosting options.
- **README:** a first draft of the structure, which I rewrote in my own words.
- I also used it to read error messages I didn't recognise.

**2. A suggestion that improved my work**
When I described the booking problem, the AI pointed out that checking whether a slot is free and then inserting is not safe when two requests arrive together. It suggested a unique partial index on `(doctor_id, start_time) WHERE status = 'booked'`, so the database settles the race and the code just turns the failure into a `409`. I tested it by sending 10 requests at once and got one `201` and nine `409`s.

**3. Where the AI was wrong or incomplete, and how I caught it**
- It told me my booking IDs were skipping because of failed inserts. I looked at the actual table, which only had one row, so the real reason my reschedule returned `404` was that the appointment I was using didn't exist. I only found that by querying the database.
- The first version of the booking handler hid the real JSON decoding error and just returned a generic message, so when requests kept failing with `400` I couldn't tell why. I found the cause (a bad struct tag) myself by reading the response and my code, and the handler now logs the decode error.
- One of the generated tests for the 1-hour rule used a clock time that failed the working-hours check first, so it would have tested the wrong rule. It was caught while rewriting the tests, not by the test run.
- Generated queries and snippets also needed fixing against real Postgres, such as an argument left out of a query and a column name that didn't match my migration. Unit tests passed with the fake store while these were still broken, which is why I say above that the SQL needs integration tests.

**4. Two decisions I made without AI, and why I trusted myself**
- **The service skeleton:** I'd already written the config, graceful shutdown, request-id and logging middleware and the JSON error format before using AI on the booking logic. I've built and run services like this at work, so I knew what I wanted and didn't need a second opinion.
- **Choosing Render over AWS:** I chose it myself, knowing AWS would take hours of networking and IAM setup for no benefit to what's being assessed. I'd done the cost and effort comparison and was comfortable with that trade-off.
