#!/usr/bin/env bash
# Online backup of the FinEstDB SQLite database.
#
# Uses sqlite3's .backup (the online backup API), which is safe against a live
# WAL-mode database — never cp/rsync the raw file while the server is running,
# that races the WAL and can produce a corrupt copy.
#
# The database holds BOTH the rebuildable dictionary and the unrebuildable
# user data (accounts, decks, review state, feedback), so the full file is
# backed up. Budget disk accordingly: the DB is 5+ GB; with default rotation
# (7 daily + 4 weekly) plan for roughly 12x the compressed size.
#
# Usage: backup-db.sh [db-path] [backup-dir]
#   db-path     default: ./finnestdb.db
#   backup-dir  default: ./backups
#
# Cron/systemd-timer this nightly; see deploy/finnestdb-backup.service.

set -euo pipefail

DB_PATH="${1:-finnestdb.db}"
BACKUP_DIR="${2:-backups}"
KEEP_DAILY="${KEEP_DAILY:-7}"
KEEP_WEEKLY="${KEEP_WEEKLY:-4}"

if [ ! -f "$DB_PATH" ]; then
    echo "error: database not found at $DB_PATH" >&2
    exit 1
fi
if ! command -v sqlite3 >/dev/null; then
    echo "error: sqlite3 CLI not installed" >&2
    exit 1
fi

mkdir -p "$BACKUP_DIR/daily" "$BACKUP_DIR/weekly"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
TMP="$BACKUP_DIR/daily/.finnestdb-$STAMP.db.tmp"
OUT="$BACKUP_DIR/daily/finnestdb-$STAMP.db.gz"

echo "backing up $DB_PATH -> $OUT"
sqlite3 "$DB_PATH" ".backup '$TMP'"

# Integrity-check the copy, not the live DB, so a bad backup never rotates a
# good one out silently.
CHECK="$(sqlite3 "$TMP" "PRAGMA integrity_check;")"
if [ "$CHECK" != "ok" ]; then
    echo "error: integrity_check on backup copy failed: $CHECK" >&2
    rm -f "$TMP"
    exit 1
fi

gzip -c "$TMP" > "$OUT"
rm -f "$TMP"

# Promote one backup per week (Sunday) into weekly retention.
if [ "$(date -u +%u)" = "7" ]; then
    cp "$OUT" "$BACKUP_DIR/weekly/"
fi

# Rotate: keep newest N in each tier.
prune() {
    local dir="$1" keep="$2"
    ls -1t "$dir"/finnestdb-*.db.gz 2>/dev/null | tail -n "+$((keep + 1))" | while read -r old; do
        echo "pruning $old"
        rm -f "$old"
    done
}
prune "$BACKUP_DIR/daily" "$KEEP_DAILY"
prune "$BACKUP_DIR/weekly" "$KEEP_WEEKLY"

echo "backup complete: $(du -h "$OUT" | cut -f1) $(basename "$OUT")"
