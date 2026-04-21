#!/bin/bash
while true; do
    clear
    echo "=== NODES ==="
    curl -s http://localhost:8080/nodes | python3 -c "
import json,sys
nodes = json.load(sys.stdin) or []
for n in nodes:
    bar = '#' * int(n['used_cpu'] / n['total_cpu'] * 20) if n['total_cpu'] > 0 else ''
    print(f\"  {n['id']:10} {n['status']:8} CPU:[{bar:<20}] {n['used_cpu']}/{n['total_cpu']}m\")
"
    echo ""
    echo "=== PODS ==="
    curl -s http://localhost:8080/pods | python3 -c "
import json,sys
pods = json.load(sys.stdin) or []
for p in pods:
    print(f\"  {p['id'][:8]}  {p['image']:10} {p['status']:10} node={p['node_id']}\")
"
    sleep 2
done