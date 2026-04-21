package store

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "github.com/Dinuka-Nonis/mini-orchestrator/types"
	"fmt"
	"time"
	"log"
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

func (s *Store) UpsertNode(n *types.Node) error {
	_, err := s.db.Exec(`
		INSERT INTO nodes (id,addr,total_cpu,total_mem,status,last_seen)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			addr=excluded.addr, status=excluded.status, last_seen=excluded.last_seen`,
		n.ID, n.Addr, n.TotalCPU, n.TotalMem, n.Status, time.Now())
	return err
}

func (s *Store) TouchNode(id string) {
	s.db.Exec(`UPDATE nodes SET last_seen=? WHERE id=?`, time.Now(), id)
}

func (s *Store) ListPodsForNode(nodeID string) ([]types.Pod, error) {
	rows, err := s.db.Query(
		`SELECT id,image,cpu,memory,node_id,container_id,status,created_at
		 FROM pods WHERE node_id=? AND status NOT IN ('stopped','pending')`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pods []types.Pod
	for rows.Next() {
		var p types.Pod
		rows.Scan(&p.ID, &p.Image, &p.CPU, &p.Memory,
			&p.NodeID, &p.ContainerID, &p.Status, &p.CreatedAt)
		pods = append(pods, p)
	}
	return pods, nil
}

func (s *Store) UpdatePodContainerID(id, containerID string) {
	s.db.Exec(`UPDATE pods SET container_id=? WHERE id=?`, containerID, id)
}

func (s *Store) ListPodsByStatus(status types.PodStatus) ([]types.Pod, error) {
	rows, err := s.db.Query(
		`SELECT id,image,cpu,memory,node_id,container_id,status,created_at
		 FROM pods WHERE status=?`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pods []types.Pod
	for rows.Next() {
		var p types.Pod
		rows.Scan(&p.ID, &p.Image, &p.CPU, &p.Memory,
			&p.NodeID, &p.ContainerID, &p.Status, &p.CreatedAt)
		pods = append(pods, p)
	}
	return pods, nil
}

func (s *Store) ListReadyNodes() ([]*types.Node, error) {
	rows, err := s.db.Query(
		`SELECT id,addr,total_cpu,total_mem,used_cpu,used_mem,last_seen,status
		 FROM nodes WHERE status='ready'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []*types.Node
	for rows.Next() {
		var n types.Node
		rows.Scan(&n.ID, &n.Addr, &n.TotalCPU, &n.TotalMem,
			&n.UsedCPU, &n.UsedMem, &n.LastSeen, &n.Status)
		nodes = append(nodes, &n)
	}
	return nodes, nil
}

func (s *Store) AssignPod(podID, nodeID string) error {
	_, err := s.db.Exec(
		`UPDATE pods SET node_id=?, status='scheduled' WHERE id=? AND status='pending'`,
		nodeID, podID)
	return err
}

func (s *Store) UpdatePodRunning(id, containerID string) error {
	log.Printf("[store] UpdatePodRunning id=%q containerID=%q", id, containerID)
	result, err := s.db.Exec(
		`UPDATE pods SET status='running', container_id=? WHERE id=?`,
		containerID, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	log.Printf("[store] UpdatePodRunning rows affected: %d", rows)
	return nil
}

func (s *Store) ListNodes() ([]types.Node, error) {
	rows, err := s.db.Query(
		`SELECT id,addr,total_cpu,total_mem,used_cpu,used_mem,last_seen,status
		 FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []types.Node
	for rows.Next() {
		var n types.Node
		rows.Scan(&n.ID, &n.Addr, &n.TotalCPU, &n.TotalMem,
			&n.UsedCPU, &n.UsedMem, &n.LastSeen, &n.Status)
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *Store) UpdateNodeStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE nodes SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) ResetPod(id string) error {
	// first get the pod so we know how much resource to free
	var cpu, mem int
	var nodeID string
	s.db.QueryRow(`SELECT cpu, memory, node_id FROM pods WHERE id=?`, id).
		Scan(&cpu, &mem, &nodeID)

	_, err := s.db.Exec(
		`UPDATE pods SET status='pending', node_id='', container_id='' WHERE id=?`, id)
	if err != nil {
		return err
	}
	// free the resources on the node
	if nodeID != "" {
		s.db.Exec(
			`UPDATE nodes SET used_cpu=MAX(0,used_cpu-?), used_mem=MAX(0,used_mem-?) WHERE id=?`,
			cpu, mem, nodeID)
	}
	return nil
}

func (s *Store) UpdateNodeUsage(nodeID string, cpuDelta, memDelta int) error {
	_, err := s.db.Exec(
		`UPDATE nodes SET used_cpu=used_cpu+?, used_mem=used_mem+? WHERE id=?`,
		cpuDelta, memDelta, nodeID)
	return err
}