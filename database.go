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
		sel_type TEXT NOT NULL,
		sel_frames INTEGER NOT NULL
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
		sel_frames INTEGER NOT NULL,
		src_video INTEGER REFERENCES videos(id),
		video_id INTEGER REFERENCES videos(id),
		label_type TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS labels (
		id INTEGER PRIMARY KEY ASC,
		set_id INTEGER REFERENCES label_sets(id),
		clip_id INTEGER REFERENCES clips(id),
		start INTEGER NOT NULL,
		end INTEGER NOT NULL
	)`)
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
