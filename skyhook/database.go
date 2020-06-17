package skyhook

import (
	_ "github.com/mattn/go-sqlite3"

	"database/sql"
	"log"
	"sync"
)

const DbDebug bool = false

var db *Database

type Database struct {
	db *sql.DB
	mu sync.Mutex
}

func init() {
	sdb, err := sql.Open("sqlite3", "./skyhook.sqlite3")
	if err != nil {
		panic(err)
	}
	db = &Database{db: sdb}

	db.Exec(`CREATE TABLE IF NOT EXISTS timelines (
		id INTEGER PRIMARY KEY ASC,
		name TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS segments (
		id INTEGER PRIMARY KEY ASC,
		timeline_id INTEGER REFERENCES timelines(id),
		name TEXT,
		frames INTEGER,
		fps REAL
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS series (
		id INTEGER PRIMARY KEY ASC,
		timeline_id INTEGER REFERENCES timelines(id),
		name TEXT,
		-- possible types:
		-- 'data': raw data
		-- 'labels': hand-labeled annotations
		-- 'outputs': query outputs
		type TEXT,
		data_type TEXT,
		-- set if type is 'labels' or 'outputs'
		src_vector TEXT,
		-- set if type is 'outputs'
		node_id INTEGER REFERENCES nodes(id),
		-- set if type is 'data' during ingestion
		percent INTEGER NOT NULL DEFAULT 100
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY ASC,
		segment_id INTEGER REFERENCES segments(id),
		series_id INTEGER REFERENCES series(id),
		start INTEGER,
		end INTEGER,
		-- video: 'mp4' or 'jpeg'
		-- others: 'json'
		format TEXT,
		-- set if video
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY ASC,
		name TEXT NOT NULL,
		parents TEXT NOT NULL,
		type TEXT NOT NULL,
		ext TEXT,
		code TEXT NOT NULL
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS vnodes (
		id INTEGER PRIMARY KEY ASC,
		node_id INTEGER REFERENCES nodes(id),
		-- input
		vector TEXT,
		-- persisted outputs
		series_id INTEGER REFERENCES series(id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS queries (
		id INTEGER PRIMARY KEY ASC,
		name TEXT NOT NULL DEFAULT '',
		outputs TEXT NOT NULL DEFAULT '',
		selector INTEGER REFERENCES nodes(id)
	)`)
}

func (this *Database) Query(q string, args ...interface{}) *Rows {
	this.mu.Lock()
	if DbDebug {
		log.Printf("[db] Query: %v", q)
	}
	rows, err := this.db.Query(q, args...)
	checkErr(err)
	return &Rows{this, true, rows}
}

func (this *Database) QueryRow(q string, args ...interface{}) *Row {
	this.mu.Lock()
	if DbDebug {
		log.Printf("[db] QueryRow: %v", q)
	}
	row := this.db.QueryRow(q, args...)
	return &Row{this, true, row}
}

func (this *Database) Exec(q string, args ...interface{}) Result {
	this.mu.Lock()
	defer this.mu.Unlock()
	if DbDebug {
		log.Printf("[db] Exec: %v", q)
	}
	result, err := this.db.Exec(q, args...)
	checkErr(err)
	return Result{result}
}

func (this *Database) Transaction(f func(tx Tx)) {
	this.mu.Lock()
	f(Tx{this})
	this.mu.Unlock()
}

type Rows struct {
	db     *Database
	locked bool
	rows   *sql.Rows
}

func (r *Rows) Close() {
	err := r.rows.Close()
	checkErr(err)
	if r.locked {
		r.db.mu.Unlock()
		r.locked = false
	}
}

func (r *Rows) Next() bool {
	hasNext := r.rows.Next()
	if !hasNext && r.locked {
		r.db.mu.Unlock()
		r.locked = false
	}
	return hasNext
}

func (r *Rows) Scan(dest ...interface{}) {
	err := r.rows.Scan(dest...)
	checkErr(err)
}

type Row struct {
	db     *Database
	locked bool
	row    *sql.Row
}

func (r Row) Scan(dest ...interface{}) {
	err := r.row.Scan(dest...)
	checkErr(err)
	if r.locked {
		r.db.mu.Unlock()
		r.locked = false
	}
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

type Tx struct {
	db *Database
}

func (tx Tx) Query(q string, args ...interface{}) Rows {
	rows, err := tx.db.db.Query(q, args...)
	checkErr(err)
	return Rows{tx.db, false, rows}
}

func (tx Tx) QueryRow(q string, args ...interface{}) Row {
	row := tx.db.db.QueryRow(q, args...)
	return Row{tx.db, false, row}
}

func (tx Tx) Exec(q string, args ...interface{}) Result {
	result, err := tx.db.db.Exec(q, args...)
	checkErr(err)
	return Result{result}
}
