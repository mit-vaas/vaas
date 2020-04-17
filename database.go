package main

import (
	_ "github.com/mattn/go-sqlite3"

	"database/sql"
)

var db *Database

type Database struct {
	db *sql.DB
}

func init() {
	sdb, err := sql.Open("sqlite3", "./skyhook.sqlite3")
	if err != nil {
		panic(err)
	}
	db = &Database{db: sdb}

	db.Exec(`CREATE TABLE IF NOT EXISTS videos (
		id INTEGER PRIMARY KEY ASC,
		name TEXT NOT NULL,
		ext TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS clips (
		id INTEGER PRIMARY KEY ASC,
		video_id INTEGER REFERENCES videos(id),
		nframes INTEGER,
		width INTEGER,
		height INTEGER
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS ops (
		id INTEGER PRIMARY KEY ASC,
		name TEXT NOT NULL,
		parents TEXT NOT NULL,
		type TEXT NOT NULL,
		ext TEXT,
		code TEXT NOT NULL,
		unit INTEGER NOT NULL
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY ASC,
		query TEXT NOT NULL,
		video_id INTEGER REFERENCES videos(id),
		ls_id INTEGER REFERENCES label_sets(id),
		type TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS label_sets (
		id INTEGER PRIMARY KEY ASC,
		name TEXT NOT NULL,
		unit INTEGER NOT NULL,
		-- annotation system should sample clips from src_video
		src_video INTEGER REFERENCES videos(id),
		-- labels.clip_id/start/end reference clips in video_id
		-- for query outputs, video_id = src_video
		-- but when annotating, we create a new video that just has the images/clips
		--     that we asked the human to label
		video_id INTEGER REFERENCES videos(id),
		label_type TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS labels (
		id INTEGER PRIMARY KEY ASC,
		set_id INTEGER REFERENCES label_sets(id),
		clip_id INTEGER REFERENCES clips(id),
		start INTEGER NOT NULL,
		end INTEGER NOT NULL,
		-- out_clip_id is only set if label_sets.label_type=video
		-- it refers to clip containing the output corresponding to input clip_id above
		out_clip_id INTEGER REFERENCES clips(id)
	)`)
/*
INSERT INTO videos VALUES (1, 'tokyo', 'jpeg');
INSERT INTO clips VALUES (1, 1, 30000, 960, 540);
INSERT INTO ops VALUES (1, 'tracker', 'v', 'track', 'python', 'TODO', 750);
INSERT INTO ops VALUES (2, 'left-to-right', 'o1', 'track', 'python', 'TODO', 750);
INSERT INTO label_sets VALUES (1, 'tokyo', 750, 1, 1, 'detection');
INSERT INTO label_sets VALUES (2, 'tracker', 750, 1, 1, 'track');
INSERT INTO labels VALUES (1, 1, 1, 0, 30000);
INSERT INTO labels VALUES (2, 2, 1, 0, 30000);
INSERT INTO nodes VALUES (1, 'o1', 1, 2, 'track');
INSERT INTO nodes VALUES (2, 'o2', 1, NULL, 'track');
*/
}

func (this *Database) Query(q string, args ...interface{}) Rows {
	rows, err := this.db.Query(q, args...)
	checkErr(err)
	return Rows{rows}
}

func (this *Database) QueryRow(q string, args ...interface{}) Row {
	row := this.db.QueryRow(q, args...)
	return Row{row}
}

func (this *Database) Exec(q string, args ...interface{}) Result {
	result, err := this.db.Exec(q, args...)
	checkErr(err)
	return Result{result}
}

type Rows struct {
	rows *sql.Rows
}

func (r Rows) Close() {
	err := r.rows.Close()
	checkErr(err)
}

func (r Rows) Next() bool {
	return r.rows.Next()
}

func (r Rows) Scan(dest ...interface{}) {
	err := r.rows.Scan(dest...)
	checkErr(err)
}

type Row struct {
	row *sql.Row
}

func (r Row) Scan(dest ...interface{}) {
	err := r.row.Scan(dest...)
	checkErr(err)
}

type Result struct {
	result sql.Result
}

func (r Result) LastInsertId() int {
	id, err := r.result.LastInsertId()
	checkErr(err)
	return int(id)
}

func (r Result) RowsAffected() int {
	count, err := r.result.RowsAffected()
	checkErr(err)
	return int(count)
}
