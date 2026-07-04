package store

// This file exposes narrow seeding/inspection helpers for cross-package tests
// (notably internal/api, which cannot reach the unexported *DB.db handle or the
// store package's own *_test.go helpers). They perform direct table writes/reads
// with no product logic and are not used by any runtime code path. Keeping them
// in a normal .go file — rather than an _test.go — is required so the api test
// package can call them.

// InsertFormForTest inserts a single (form, lemma, pos, lang) row into the forms
// table. Test-only seeding helper.
func (d *DB) InsertFormForTest(form, lemma, pos, lang string) error {
	_, err := d.db.Exec(
		`INSERT INTO forms (form, lemma, pos, lang) VALUES (?, ?, ?, ?)`,
		form, lemma, pos, lang,
	)
	return err
}

// InsertLemmaForTest inserts a single (lemma, pos, gloss, lang) row into the
// lemmas table. Test-only seeding helper.
func (d *DB) InsertLemmaForTest(lemma, pos, gloss, lang string) error {
	_, err := d.db.Exec(
		`INSERT INTO lemmas (lemma, pos, gloss, lang) VALUES (?, ?, ?, ?)`,
		lemma, pos, gloss, lang,
	)
	return err
}

// HasCardForTest reports whether the user has a review card for the given
// (lang, lemma, pos). Test-only inspection helper.
func (d *DB) HasCardForTest(userID int64, lang, lemma, pos string) (bool, error) {
	var n int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE user_id = ? AND lang = ? AND lemma = ? AND pos = ?`,
		userID, lang, lemma, pos,
	).Scan(&n)
	return n > 0, err
}
