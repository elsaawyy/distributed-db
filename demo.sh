#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# demo.sh — Full end-to-end demo of the Distributed Database System
# Prerequisites: all 3 nodes must be running (./start-all.sh)
# ─────────────────────────────────────────────────────────────────────────────

MASTER="http://localhost:8080"
WORKER_GO="http://localhost:8081"
WORKER_PY="http://localhost:8082"

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; NC='\033[0m'; BOLD='\033[1m'

step() { echo -e "\n${BOLD}${CYAN}▶ $1${NC}"; }
ok()   { echo -e "${GREEN}✓ $1${NC}"; }
req()  { echo -e "  curl: $1"; eval "$1" | python3 -m json.tool 2>/dev/null || echo "$1"; }

echo -e "${BOLD}"
echo "╔══════════════════════════════════════════════════════╗"
echo "║      Distributed Database System — Full Demo         ║"
echo "╚══════════════════════════════════════════════════════╝"
echo -e "${NC}"

# ─── Health Checks ────────────────────────────────────────────────────────────
step "1. Health Check — All Nodes"
req "curl -s $MASTER/health"
req "curl -s $WORKER_GO/health"
req "curl -s $WORKER_PY/health"

# ─── Create Database ──────────────────────────────────────────────────────────
step "2. Create Database 'company'"
req "curl -s -X POST $MASTER/create-db -H 'Content-Type: application/json' -d '{\"name\":\"company\"}'"

# ─── Create Table ─────────────────────────────────────────────────────────────
step "3. Create Table 'employees'"
req "curl -s -X POST $MASTER/create-table \
  -H 'Content-Type: application/json' \
  -d '{
    \"database\": \"company\",
    \"table\": \"employees\",
    \"columns\": [
      {\"name\": \"name\",       \"type\": \"string\", \"required\": true},
      {\"name\": \"department\", \"type\": \"string\", \"required\": true},
      {\"name\": \"salary\",     \"type\": \"float\",  \"required\": true},
      {\"name\": \"age\",        \"type\": \"int\",    \"required\": false}
    ]
  }'"

# ─── Insert Records ───────────────────────────────────────────────────────────
step "4. Insert Employees"

insert_employee() {
  curl -s -X POST $MASTER/insert \
    -H "Content-Type: application/json" \
    -d "{\"database\":\"company\",\"table\":\"employees\",\"fields\":$1}"
}

R1=$(insert_employee '{"name":"Alice Smith","department":"Engineering","salary":95000,"age":30}')
echo "$R1" | python3 -m json.tool
ID1=$(echo "$R1" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null)

insert_employee '{"name":"Bob Jones","department":"Marketing","salary":72000,"age":25}' | python3 -m json.tool
insert_employee '{"name":"Carol White","department":"Engineering","salary":105000,"age":35}' | python3 -m json.tool
insert_employee '{"name":"Dave Brown","department":"HR","salary":65000,"age":28}' | python3 -m json.tool
insert_employee '{"name":"Eve Davis","department":"Engineering","salary":88000,"age":22}' | python3 -m json.tool

# ─── Select All ───────────────────────────────────────────────────────────────
step "5. Select All Employees"
req "curl -s '$MASTER/select?database=company&table=employees'"

# ─── Select with WHERE ────────────────────────────────────────────────────────
step "6. Select Engineering Department Only"
req "curl -s '$MASTER/select?database=company&table=employees&where_department=Engineering'"

# ─── Update ───────────────────────────────────────────────────────────────────
step "7. Update Alice's Salary"
if [ -n "$ID1" ]; then
  req "curl -s -X PUT $MASTER/update \
    -H 'Content-Type: application/json' \
    -d '{\"database\":\"company\",\"table\":\"employees\",\"id\":\"$ID1\",\"fields\":{\"salary\":102000}}'"
else
  echo "  (Skipping update — could not extract ID)"
fi

# ─── Search ───────────────────────────────────────────────────────────────────
step "8. Search for 'Smith' in name column"
req "curl -s '$MASTER/search?database=company&table=employees&column=name&value=Smith'"

# ─── Go Worker Analytics (Special Task) ──────────────────────────────────────
step "9. Go Worker — Analytics (Special Task)"
req "curl -s '$WORKER_GO/analytics?database=company&table=employees'"

# ─── Python Worker Transform (Special Task) ───────────────────────────────────
step "10. Python Worker — Transform: Filter Engineers aged > 25, sort by salary desc"
req "curl -s -X POST $WORKER_PY/transform \
  -H 'Content-Type: application/json' \
  -d '{
    \"database\": \"company\",
    \"table\": \"employees\",
    \"transformations\": [
      {\"type\": \"filter\",  \"column\": \"department\", \"operator\": \"eq\",   \"value\": \"Engineering\"},
      {\"type\": \"filter\",  \"column\": \"age\",        \"operator\": \"gt\",   \"value\": \"25\"},
      {\"type\": \"project\", \"columns\": [\"name\", \"salary\", \"age\"]},
      {\"type\": \"cast\",    \"column\": \"salary\",     \"to_type\": \"float\"},
      {\"type\": \"sort\",    \"column\": \"salary\",     \"order\": \"desc\"}
    ]
  }'"

# ─── Read from Worker (Fault-Tolerant) ────────────────────────────────────────
step "11. Read directly from Go Worker (fault-tolerant reads)"
req "curl -s '$WORKER_GO/select?database=company&table=employees'"

step "12. Read directly from Python Worker (fault-tolerant reads)"
req "curl -s '$WORKER_PY/select?database=company&table=employees'"

# ─── Master Status ────────────────────────────────────────────────────────────
step "13. Master System Status"
req "curl -s $MASTER/status"

# ─── Delete a Record ──────────────────────────────────────────────────────────
if [ -n "$ID1" ]; then
  step "14. Delete Alice's record"
  req "curl -s -X DELETE $MASTER/delete \
    -H 'Content-Type: application/json' \
    -d '{\"database\":\"company\",\"table\":\"employees\",\"id\":\"$ID1\"}'"
fi

echo -e "\n${GREEN}${BOLD}Demo complete!${NC}"
echo -e "Visit ${CYAN}http://localhost:8080/status${NC} for full system state.\n"
