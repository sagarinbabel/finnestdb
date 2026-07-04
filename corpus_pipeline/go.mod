module finnestdb/corpus_pipeline

go 1.21

replace finnestdb => ..

require (
	finnestdb v0.0.0-00010101000000-000000000000
	github.com/mattn/go-sqlite3 v1.14.44
)

require github.com/open-spaced-repetition/go-fsrs/v3 v3.3.1 // indirect
