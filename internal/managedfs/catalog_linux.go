//go:build linux

package managedfs

import (
	"context"
	"database/sql"
)

// This fixed catalog is separate from the legacy application database. Open
// never migrates it and no caller can substitute a per-operation journal.
const catalogSchema = `
CREATE TABLE identity (
 id INTEGER PRIMARY KEY CHECK(id=1), root_id TEXT NOT NULL, store_id TEXT NOT NULL,
 epoch INTEGER NOT NULL CHECK(epoch>=0), pending TEXT NOT NULL
);
CREATE TABLE objects (
 id TEXT PRIMARY KEY, proof TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('SEALED','PURGED'))
);
CREATE TABLE operations (
 epoch INTEGER PRIMARY KEY CHECK(epoch>0), id TEXT NOT NULL UNIQUE,
 kind TEXT NOT NULL CHECK(kind IN ('SEAL','PURGE')), object_id TEXT NOT NULL,
 intent TEXT NOT NULL, staged_proof TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('PENDING','STAGED','IRREVERSIBLE','COMPLETE'))
);
PRAGMA user_version=1;`

func affected(result sql.Result, err error) error {
	if err != nil {
		return ErrRecoveryRequired
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) catalogReady(ctx context.Context) error {
	var root, store, pending string
	var epoch, count, incomplete int64
	var version int
	if w.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version) != nil || version != 1 ||
		w.db.QueryRowContext(ctx, "SELECT root_id,store_id,epoch,pending FROM identity WHERE id=1").Scan(&root, &store, &epoch, &pending) != nil ||
		root != w.r.proof.RootID || store != w.r.proof.StoreID || epoch != w.control.Epoch || pending != "" ||
		w.db.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(status!='COMPLETE'),0) FROM operations").Scan(&count, &incomplete) != nil || count != epoch || incomplete != 0 {
		return ErrRecoveryRequired
	}
	return nil
}

func (w *writer) catalogObject(ctx context.Context, proof ObjectProof) error {
	var raw, state string
	if w.db.QueryRowContext(ctx, "SELECT proof,state FROM objects WHERE id=?", proof.ObjectID).Scan(&raw, &state) != nil || state != "SEALED" || raw != string(encoded(proof)) {
		return ErrRejected
	}
	return nil
}

// transaction's post-commit hook also covers acknowledgement loss: the Writer
// stays blocked even when the transaction may have committed successfully.
func (w *writer) transaction(ctx context.Context, label string, body func(*sql.Tx) error) error {
	if w.bound() != nil {
		return ErrRecoveryRequired
	}
	return w.step(ctx, label, func() error {
		tx, err := w.db.BeginTx(ctx, nil)
		if err != nil {
			return ErrRecoveryRequired
		}
		defer tx.Rollback()
		if err = body(tx); err != nil || w.step(ctx, label+".commit", tx.Commit) != nil {
			return ErrRecoveryRequired
		}
		return nil
	})
}

func (w *writer) beginCatalog(ctx context.Context, op intent, oldEpoch int64) error {
	return w.transaction(ctx, "catalog.begin", func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, "UPDATE identity SET epoch=?,pending=? WHERE id=1 AND root_id=? AND store_id=? AND epoch=? AND pending=''", w.control.Epoch, op.ID, w.r.proof.RootID, w.r.proof.StoreID, oldEpoch)); err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, "INSERT INTO operations(epoch,id,kind,object_id,intent,status) VALUES(?,?,?,?,?,'PENDING')", w.control.Epoch, op.ID, op.Kind, op.ObjectID, string(encoded(op))))
	})
}

func (w *writer) stageCatalog(ctx context.Context, op intent, proof ObjectProof) error {
	return w.transaction(ctx, "catalog.staged", func(tx *sql.Tx) error {
		return affected(tx.ExecContext(ctx, "UPDATE operations SET staged_proof=?,status='STAGED' WHERE id=? AND epoch=? AND status='PENDING' AND EXISTS(SELECT 1 FROM identity WHERE id=1 AND epoch=? AND pending=?)", string(encoded(proof)), op.ID, w.control.Epoch, w.control.Epoch, op.ID))
	})
}

func (w *writer) irreversibleCatalog(ctx context.Context, op intent) error {
	return w.transaction(ctx, "catalog.irreversible", func(tx *sql.Tx) error {
		return affected(tx.ExecContext(ctx, "UPDATE operations SET status='IRREVERSIBLE' WHERE id=? AND epoch=? AND status='STAGED' AND EXISTS(SELECT 1 FROM identity WHERE id=1 AND epoch=? AND pending=?)", op.ID, w.control.Epoch, w.control.Epoch, op.ID))
	})
}

func (w *writer) completeCatalog(ctx context.Context, op intent, proof ObjectProof) error {
	return w.transaction(ctx, "catalog.complete", func(tx *sql.Tx) error {
		if op.Kind == "SEAL" {
			if err := affected(tx.ExecContext(ctx, "INSERT INTO objects(id,proof,state) VALUES(?,?,'SEALED')", proof.ObjectID, string(encoded(proof)))); err != nil {
				return err
			}
		} else if err := affected(tx.ExecContext(ctx, "UPDATE objects SET state='PURGED' WHERE id=? AND proof=? AND state='SEALED'", proof.ObjectID, string(encoded(proof)))); err != nil {
			return err
		}
		previous := "STAGED"
		if op.Kind == "PURGE" {
			previous = "IRREVERSIBLE"
		}
		if err := affected(tx.ExecContext(ctx, "UPDATE operations SET status='COMPLETE' WHERE id=? AND epoch=? AND status=?", op.ID, w.control.Epoch, previous)); err != nil {
			return err
		}
		return affected(tx.ExecContext(ctx, "UPDATE identity SET pending='' WHERE id=1 AND epoch=? AND pending=?", w.control.Epoch, op.ID))
	})
}
