package store

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "github.com/Dinuka-Nonis/mini-orchestrator/types"
	"fmt"
)

type Store struct{ db *sql.DB }

func New(path string) *Store {
    db, err := sql.Open("sqlite3", path)
    if err != nil { panic(err) }
    db.Exec(`CREATE TABLE IF NOT EXISTS pods (
        id TEXT PRIMARY KEY, image TEXT, cpu INTEGER,
        memory INTEGER, node_id TEXT DEFAULT '',
        container_id TEXT DEFAULT '',
        status TEXT DEFAULT 'pending',
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
    db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
        id TEXT PRIMARY KEY, addr TEXT,
        total_cpu INTEGER, total_mem INTEGER,
        used_cpu INTEGER DEFAULT 0, used_mem INTEGER DEFAULT 0,
        last_seen DATETIME, status TEXT DEFAULT 'ready')`)
    return &Store{db: db}
}

func (s *Store) CreatePod(p *types.Pod) error {
    _, err := s.db.Exec(
        `INSERT INTO pods (id,image,cpu,memory,status,created_at)
         VALUES (?,?,?,?,?,?)`,
        p.ID, p.Image, p.CPU, p.Memory, p.Status, p.CreatedAt)
    return err
}

func (s *Store) ListPods() ([]types.Pod, error) {
    rows, err := s.db.Query(`SELECT id,image,cpu,memory,node_id,container_id,status,created_at FROM pods`)
    if err != nil { return nil, err }
    defer rows.Close()
    var pods []types.Pod
    for rows.Next() {
        var p types.Pod
        rows.Scan(&p.ID,&p.Image,&p.CPU,&p.Memory,&p.NodeID,&p.ContainerID,&p.Status,&p.CreatedAt)
        pods = append(pods, p)
    }
    return pods, nil
}

func (s *Store) UpdatePodStatus(id string, status types.PodStatus) error {
	res, err := s.db.Exec(`UPDATE pods SET status=? WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("pod not found")
	}
	return nil
}