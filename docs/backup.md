# MySQL backup strategy

The Mutaba'ah Yaumiyah stores everything that matters in MySQL — user accounts,
task definitions, assignments, and completion history. The app itself is
stateless beyond the templates baked into the binary, so the backup story is
really a MySQL story.

## Goals

- **RPO** (data loss tolerance): ≤ 24 hours.
- **RTO** (recovery time): ≤ 1 hour from a cold backup.
- Backups must be **off-host** — losing the VM should not lose the backups.

## Recommended baseline

A nightly logical dump uploaded to object storage, retained for 30 days, plus
an annual snapshot retained indefinitely.

### Nightly dump

Run `mysqldump` with single-transaction so the dump is consistent against an
InnoDB store without taking write locks:

```bash
mysqldump \
  --single-transaction \
  --quick \
  --routines --triggers --events \
  --set-gtid-purged=OFF \
  --databases tracker \
  | gzip -9 \
  > "tracker-$(date -u +%Y%m%dT%H%M%SZ).sql.gz"
```

Then upload to S3/GCS/B2:

```bash
aws s3 cp tracker-*.sql.gz s3://my-backups/tracker/ --storage-class STANDARD_IA
```

Wire it up as a host cron job (or a Kubernetes CronJob) running at e.g. 02:15
UTC. Send the exit code to your monitoring (Healthchecks.io, Cronitor, etc.) so
silent failures don't accumulate.

### Retention

A simple lifecycle policy on the bucket:

- 30 days → kept hot.
- After 30 days → moved to colder storage (Glacier / Coldline).
- After 365 days → deleted, except the first dump of each year, which is kept
  as a yearly archive.

## Restore drill

Test the restore quarterly — an untested backup is a wish, not a backup.

```bash
gunzip -c tracker-YYYYMMDDTHHMMSSZ.sql.gz \
  | mysql -h 127.0.0.1 -u root -p
```

Then point a staging instance of the app at the restored database and confirm
login + dashboard render. Document the wall-clock time it took; that's your
real RTO.

## Higher-RPO option (point-in-time recovery)

If 24-hour data loss is too much, enable binary logging on MySQL and ship the
binlogs alongside the full dump. Restore = replay the most recent full dump,
then apply binlogs up to the desired position.

This is overkill for v1 but worth re-evaluating once the user base grows past
a single household.

## What is _not_ backed up

- Session rows in the `sessions` table — losing them just forces users to
  re-login. They're regenerable, so excluding them from the dump is fine if
  you want to keep dumps smaller (`--ignore-table=tracker.sessions`).
- The application binary — built from source; the Docker image registry is
  the source of truth.
- Static assets — they're committed to the repo and baked into the image.
