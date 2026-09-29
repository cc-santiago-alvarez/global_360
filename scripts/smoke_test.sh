#!/usr/bin/env bash
# Smoke test for every endpoint of the Phase 1 API.
#
# It creates companies, users, roles and catalog entries, so run it against a
# disposable database. Requires curl and jq.
#
# Usage:
#   BASE_URL=http://localhost:8080 ADMIN_EMAIL=... ADMIN_PASSWORD=... scripts/smoke_test.sh
#
# ADMIN_EMAIL/ADMIN_PASSWORD default to SEED_SUPERADMIN_EMAIL/SEED_SUPERADMIN_PASSWORD from .env.
# The run assumes MAX_FAILED_LOGIN_ATTEMPTS (default 5) for the blocking scenario.

set -uo pipefail

cd "$(dirname "$0")/.."

# env_value KEY: environment first, then .env. The file is parsed, not sourced,
# because values such as MONGO_URI contain '&'.
env_value() {
	if [[ -n ${!1:-} ]]; then
		printf '%s' "${!1}"
	elif [[ -f .env ]]; then
		grep -E "^$1=" .env | tail -n 1 | cut -d= -f2- | sed -E "s/^\"(.*)\"$/\1/; s/^'(.*)'$/\1/"
	fi
}

PORT=$(env_value PORT)
BASE_URL=${BASE_URL:-http://localhost:${PORT:-8080}}
ADMIN_EMAIL=${ADMIN_EMAIL:-$(env_value SEED_SUPERADMIN_EMAIL)}
ADMIN_PASSWORD=${ADMIN_PASSWORD:-$(env_value SEED_SUPERADMIN_PASSWORD)}
MAX_FAILED=$(env_value MAX_FAILED_LOGIN_ATTEMPTS)
MAX_FAILED=${MAX_FAILED:-5}
API=$BASE_URL/api/v1

if [[ -z $ADMIN_EMAIL || -z $ADMIN_PASSWORD ]]; then
	echo "ADMIN_EMAIL and ADMIN_PASSWORD are required" >&2
	exit 2
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

RUN=$(date +%s)
LETTERS=ABCDEFGHIJKLMNOPQRSTUVWXYZ
COUNTRY_CODE=Q${LETTERS:RANDOM%26:1}
CURRENCY_CODE=Q${LETTERS:RANDOM%26:1}${LETTERS:RANDOM%26:1}
PASSWORD='Sm0ke-Pass-2026!'
NEW_PASSWORD='Sm0ke-Pass-2027!'

PASS=0
FAIL=0
FAILURES=()
STATUS=
BODY=
TOKEN=

if [[ -t 1 ]]; then GREEN=$'\e[32m' RED=$'\e[31m' BOLD=$'\e[1m' RESET=$'\e[0m'; else GREEN= RED= BOLD= RESET=; fi

section() { printf '\n%s== %s ==%s\n' "$BOLD" "$1" "$RESET"; }

record() { # ok|fail description detail
	if [[ $1 == ok ]]; then
		PASS=$((PASS + 1))
		printf '  %sPASS%s %s\n' "$GREEN" "$RESET" "$2"
	else
		FAIL=$((FAIL + 1))
		FAILURES+=("$2 -> $3")
		printf '  %sFAIL%s %s -> %s\n' "$RED" "$RESET" "$2" "$3"
	fi
}

# req METHOD PATH EXPECTED_STATUS DESCRIPTION [JSON_BODY]
# Uses $TOKEN when set. Leaves the response in $STATUS and $BODY.
req() {
	local method=$1 path=$2 expected=$3 desc=$4 body=${5-}
	local args=(-s -o "$TMP/body" -D "$TMP/headers" -w '%{http_code}' -X "$method" "$path")
	[[ -n $TOKEN ]] && args+=(-H "Authorization: Bearer $TOKEN")
	[[ -n $body ]] && args+=(-H 'Content-Type: application/json' --data "$body")
	STATUS=$(curl "${args[@]}")
	BODY=$(cat "$TMP/body")
	if [[ $STATUS != "$expected" ]]; then
		record fail "$method ${path#"$BASE_URL"} $desc" "HTTP $STATUS, expected $expected: ${BODY:0:300}"
		return 1
	fi
	# Every error must use the {"error":{"code","message","request_id"}} envelope.
	if ((STATUS >= 400)) && ! jq -e '.error.code and .error.message and .error.request_id' <<<"$BODY" >/dev/null 2>&1; then
		record fail "$method ${path#"$BASE_URL"} $desc" "HTTP $STATUS without error envelope: ${BODY:0:300}"
		return 1
	fi
	record ok "$method ${path#"$BASE_URL"} $desc [$STATUS]"
}

# expect DESCRIPTION JQ_FILTER [jq args...]: asserts on the last response body.
expect() {
	local desc=$1 filter=$2
	shift 2
	if jq -e "$@" "$filter" <<<"$BODY" >/dev/null 2>&1; then
		record ok "  └ $desc"
	else
		record fail "  └ $desc" "filter '$filter' failed on: ${BODY:0:300}"
	fi
}

field() { jq -r "$1" <<<"$BODY"; }

login() { # email password -> sets $BODY; prints nothing
	TOKEN='' req POST "$API/auth/login" "${3:-200}" "${4:-login $1}" "$(jq -nc --arg e "$1" --arg p "$2" '{email:$e,password:$p}')"
}

user_body() { # email doc_number company_id(or empty) first_name
	jq -nc --arg e "$1" --arg d "$2" --arg c "$3" --arg f "$4" --arg country "$CO_ID" --arg p "$PASSWORD" '{
		email: $e, password: $p, company_id: (if $c == "" then null else $c end),
		person: {document_type: "CC", document_number: $d, document_country_id: $country,
			first_names: $f, last_names: "Smoke", email: $e, phone: "+57 300 000 0000"}}'
}

# ---------------------------------------------------------------------------
section "Public endpoints"

req GET "$BASE_URL/health" 200 "liveness"
expect "status ok" '.status == "ok"'
req GET "$BASE_URL/ready" 200 "readiness"
expect "database up" '.database == "up"'
req GET "$API/ping" 200 "ping"
expect "pong" '.message == "pong"'
if grep -qi '^x-request-id: .\+' "$TMP/headers"; then record ok "  └ X-Request-ID header generated"; else record fail "  └ X-Request-ID header generated" "missing header"; fi
curl -s -o /dev/null -D "$TMP/headers" -H 'X-Request-ID: smoke-req-1' "$API/ping"
if grep -qi '^x-request-id: smoke-req-1' "$TMP/headers"; then record ok "  └ X-Request-ID propagated"; else record fail "  └ X-Request-ID propagated" "not echoed"; fi
req GET "$API/does-not-exist" 404 "unknown route"
expect "code not_found" '.error.code == "not_found"'
req DELETE "$API/ping" 405 "wrong method"

# ---------------------------------------------------------------------------
section "Auth: login, me, refresh rotation"

req POST "$API/auth/login" 400 "without body"
req POST "$API/auth/login" 400 "without password" '{"email":"someone@test.com"}'
expect "message names the field" '.error.message | contains("password")'
req POST "$API/auth/login" 400 "malformed JSON" '{"email":'
login "$ADMIN_EMAIL" "wrong-password-123" 401 "wrong password"
expect "generic message" '.error.message == "invalid email or password"'
login "nobody-$RUN@test.com" "whatever-123" 401 "unknown email"
expect "same generic message" '.error.message == "invalid email or password"'
login "$ADMIN_EMAIL" "$ADMIN_PASSWORD" 200 "superadmin"
expect "bearer token pair" '.token_type == "Bearer" and (.access_token | length > 20) and (.refresh_token | length > 20)'
SA=$(field .access_token)
SA_REFRESH=$(field .refresh_token)

req GET "$API/auth/me" 401 "without token"
TOKEN=garbage req GET "$API/auth/me" 401 "with invalid token"
TOKEN=$SA req GET "$API/auth/me" 200 "superadmin"
expect "email matches" '.user.email == $e' --arg e "$(tr 'A-Z' 'a-z' <<<"$ADMIN_EMAIL")"
expect "has every global permission" '.global_permissions | length == 14'
expect "no password hash exposed" '[.. | objects | has("password_hash")] | any | not'
SA_ID=$(field .user.id)

req POST "$API/auth/refresh" 400 "without refresh_token" '{}'
req POST "$API/auth/refresh" 401 "unknown refresh token" '{"refresh_token":"not-a-real-token"}'
req POST "$API/auth/refresh" 200 "rotates token" "{\"refresh_token\":\"$SA_REFRESH\"}"
ROTATED=$(field .refresh_token)
expect "new refresh token differs" '.refresh_token != $old' --arg old "$SA_REFRESH"
req POST "$API/auth/refresh" 401 "reusing a rotated token" "{\"refresh_token\":\"$SA_REFRESH\"}"
req POST "$API/auth/refresh" 401 "reuse revoked the whole session family" "{\"refresh_token\":\"$ROTATED\"}"

login "$ADMIN_EMAIL" "$ADMIN_PASSWORD" 200 "superadmin again"
SA=$(field .access_token)
SA_REFRESH=$(field .refresh_token)
TOKEN=$SA

# ---------------------------------------------------------------------------
section "Catalog: countries and currencies"

req GET "$API/countries" 200 "list"
expect "CO and CR seeded" '[.items[].iso_code] | contains(["CO","CR"])'
CO_ID=$(field '.items[] | select(.iso_code=="CO") | .id')
CR_ID=$(field '.items[] | select(.iso_code=="CR") | .id')
req GET "$API/countries?active=true" 200 "only active"
expect "all active" 'all(.items[]; .active)'
req GET "$API/countries?active=maybe" 400 "invalid boolean filter"
req POST "$API/countries" 201 "create $COUNTRY_CODE" "{\"iso_code\":\"$COUNTRY_CODE\",\"name\":\"Smoke Land\"}"
expect "active by default" '.active == true and .iso_code == $c' --arg c "$COUNTRY_CODE"
NEW_COUNTRY=$(field .id)
req POST "$API/countries" 409 "duplicate iso_code" "{\"iso_code\":\"$COUNTRY_CODE\",\"name\":\"Again\"}"
req POST "$API/countries" 400 "invalid iso_code" '{"iso_code":"XYZ1","name":"Bad"}'
req POST "$API/countries" 400 "missing name" '{"iso_code":"QQ"}'
req PATCH "$API/countries/$NEW_COUNTRY" 200 "rename" '{"name":"Smoke Land Renamed"}'
expect "renamed" '.name == "Smoke Land Renamed"'
req PATCH "$API/countries/$NEW_COUNTRY" 200 "deactivate" '{"active":false}'
expect "inactive" '.active == false'
req PATCH "$API/countries/does-not-exist" 404 "unknown id" '{"name":"x"}'

req GET "$API/currencies" 200 "list"
expect "COP, CRC and USD seeded" '[.items[].iso_code] | contains(["COP","CRC","USD"])'
COP_ID=$(field '.items[] | select(.iso_code=="COP") | .id')
CRC_ID=$(field '.items[] | select(.iso_code=="CRC") | .id')
req POST "$API/currencies" 201 "create $CURRENCY_CODE" "{\"iso_code\":\"$CURRENCY_CODE\",\"name\":\"Smoke Coin\",\"symbol\":\"S\$\",\"decimals\":2}"
NEW_CURRENCY=$(field .id)
req POST "$API/currencies" 409 "duplicate iso_code" "{\"iso_code\":\"$CURRENCY_CODE\",\"name\":\"Again\",\"symbol\":\"A\"}"
req POST "$API/currencies" 400 "decimals out of range" '{"iso_code":"QZZ","name":"Bad","symbol":"B","decimals":9}'
req POST "$API/currencies" 400 "missing symbol" '{"iso_code":"QZZ","name":"Bad"}'
req PATCH "$API/currencies/$NEW_CURRENCY" 200 "update symbol and decimals" '{"symbol":"SC","decimals":0}'
expect "updated" '.symbol == "SC" and .decimals == 0'
req PATCH "$API/currencies/$NEW_CURRENCY" 200 "deactivate" '{"active":false}'
req PATCH "$API/currencies/does-not-exist" 404 "unknown id" '{"name":"x"}'

# ---------------------------------------------------------------------------
section "Companies"

company_body() { # legal_name doc country currency type
	jq -nc --arg n "$1" --arg d "$2" --arg c "$3" --arg m "$4" --arg t "$5" \
		'{legal_name:$n, trade_name:$n, document_type:"NIT", document_number:$d, country_id:$c,
		  billing_currency_id:$m, company_type:$t, contact_email:"ops@smoke.test"}'
}

req POST "$API/companies" 201 "create client company" "$(company_body "Smoke Client $RUN SAS" "SMK-A-$RUN" "$CO_ID" "$COP_ID" client)"
expect "starts pending_validation" '.status == "pending_validation"'
COMPANY_A=$(field .id)
req POST "$API/companies" 409 "duplicate document" "$(company_body "Other" "SMK-A-$RUN" "$CO_ID" "$COP_ID" client)"
req POST "$API/companies" 400 "invalid company_type" "$(company_body "Bad" "SMK-X1-$RUN" "$CO_ID" "$COP_ID" alien)"
req POST "$API/companies" 400 "unknown country" "$(company_body "Bad" "SMK-X2-$RUN" "does-not-exist" "$COP_ID" client)"
req POST "$API/companies" 400 "inactive country" "$(company_body "Bad" "SMK-X3-$RUN" "$NEW_COUNTRY" "$COP_ID" client)"
req POST "$API/companies" 400 "inactive currency" "$(company_body "Bad" "SMK-X4-$RUN" "$CO_ID" "$NEW_CURRENCY" client)"
req POST "$API/companies" 400 "missing legal_name" '{"document_type":"NIT"}'
req POST "$API/companies" 201 "create provider company" "$(company_body "Smoke Provider $RUN SA" "SMK-B-$RUN" "$CR_ID" "$CRC_ID" provider)"
COMPANY_B=$(field .id)

req GET "$API/companies" 200 "list"
expect "contains both companies" '[.items[].id] | contains([$a,$b])' --arg a "$COMPANY_A" --arg b "$COMPANY_B"
req GET "$API/companies?status=pending_validation&company_type=client" 200 "filter by status and type"
expect "only pending clients" 'all(.items[]; .status == "pending_validation" and .company_type == "client")'
req GET "$API/companies?page_size=1" 200 "pagination"
expect "one item, page metadata" '(.items | length) == 1 and .page_size == 1 and .page == 1 and .total >= 2'
req GET "$API/companies?page_size=500" 200 "page_size capped"
expect "page_size 100" '.page_size == 100'
req GET "$API/companies?status=bogus" 400 "invalid status filter"
req GET "$API/companies?page=abc" 400 "invalid page"
req GET "$API/companies/$COMPANY_A" 200 "get"
expect "right company" '.id == $a and .document_number == $d' --arg a "$COMPANY_A" --arg d "SMK-A-$RUN"
req GET "$API/companies/does-not-exist" 404 "unknown id"
req PATCH "$API/companies/$COMPANY_A" 200 "update" '{"trade_name":"Smoke Client Renamed","contact_phone":"+57 300 111 2222"}'
expect "updated" '.trade_name == "Smoke Client Renamed" and .contact_phone == "+57 300 111 2222"'
req PATCH "$API/companies/$COMPANY_A" 400 "inactive billing currency" "{\"billing_currency_id\":\"$NEW_CURRENCY\"}"
req PATCH "$API/companies/$COMPANY_A" 400 "invalid company_type" '{"company_type":"alien"}'
req PATCH "$API/companies/does-not-exist" 404 "unknown id" '{"trade_name":"x"}'
req PATCH "$API/companies/$COMPANY_A/status" 400 "invalid transition pending -> suspended" '{"status":"suspended"}'
req PATCH "$API/companies/$COMPANY_A/status" 400 "invalid status" '{"status":"bogus"}'
req PATCH "$API/companies/$COMPANY_A/status" 400 "missing status" '{}'
req PATCH "$API/companies/$COMPANY_A/status" 200 "activate client" '{"status":"active"}'
expect "active" '.status == "active"'
req PATCH "$API/companies/$COMPANY_B/status" 200 "activate provider" '{"status":"active"}'

# ---------------------------------------------------------------------------
section "Users"

ADMIN_EMAIL_A="admin.$RUN@smoke-a.test"
req POST "$API/users" 201 "create company admin" "$(user_body "$ADMIN_EMAIL_A" "SMK-U1-$RUN" "$COMPANY_A" Ana)"
expect "pending_verification, bound to company" '.status == "pending_verification" and .company_id == $a' --arg a "$COMPANY_A"
expect "no password hash exposed" '[.. | objects | has("password_hash")] | any | not'
ADMIN_A=$(field .id)
req POST "$API/users" 409 "duplicate email" "$(user_body "$ADMIN_EMAIL_A" "SMK-U9-$RUN" "$COMPANY_A" Dup)"
req POST "$API/users" 400 "short password" "$(user_body "short.$RUN@smoke-a.test" "SMK-U8-$RUN" "$COMPANY_A" Short | jq -c '.password = "short"')"
req POST "$API/users" 400 "missing person" "{\"email\":\"np.$RUN@smoke-a.test\",\"password\":\"$PASSWORD\"}"
req POST "$API/users" 400 "unknown company" "$(user_body "nc.$RUN@smoke-a.test" "SMK-U7-$RUN" does-not-exist Nc)"
req POST "$API/users" 400 "invalid email" "$(user_body "not-an-email" "SMK-U6-$RUN" "$COMPANY_A" Bad)"
login "$ADMIN_EMAIL_A" "$PASSWORD" 403 "pending user cannot log in"

req GET "$API/users" 200 "list"
expect "has items" '.total >= 2'
req GET "$API/users?company_id=$COMPANY_A" 200 "filter by company"
expect "only company A" '.total == 1 and .items[0].id == $u' --arg u "$ADMIN_A"
req GET "$API/users?status=bogus" 400 "invalid status filter"
req GET "$API/users/$ADMIN_A" 200 "get"
expect "includes person" '.person.first_names == "Ana" and .person.document_number == $d' --arg d "SMK-U1-$RUN"
req GET "$API/users/does-not-exist" 404 "unknown id"
req PATCH "$API/users/$ADMIN_A" 200 "update person data" '{"first_names":"Ana Maria","phone":"+57 311 111 1111"}'
expect "updated" '.person.first_names == "Ana Maria" and .person.phone == "+57 311 111 1111"'
req PATCH "$API/users/$ADMIN_A/status" 400 "invalid status" '{"status":"bogus"}'
req PATCH "$API/users/$ADMIN_A/status" 200 "activate" '{"status":"active"}'
expect "active" '.status == "active"'
req PATCH "$API/users/$ADMIN_A/status" 400 "already active" '{"status":"active"}'
req PATCH "$API/users/$SA_ID/status" 400 "own status" '{"status":"inactive"}'
req PATCH "$API/users/does-not-exist/status" 404 "unknown id" '{"status":"active"}'

# ---------------------------------------------------------------------------
section "Roles and permissions"

req GET "$API/permissions" 200 "list permissions"
expect "14 permissions" '.items | length == 14'
req GET "$API/roles" 200 "list roles"
expect "8 system roles seeded" '[.items[] | select(.is_system)] | length == 8'
role_id() { jq -r --arg n "$1" '.items[] | select(.name == $n) | .id' <<<"$ROLES"; }
ROLES=$BODY
SUPERADMIN_ROLE=$(role_id superadmin)
CLIENT_ADMIN_ROLE=$(role_id client_admin)
CLIENT_OPERATOR_ROLE=$(role_id client_operator)
PROVIDER_ADMIN_ROLE=$(role_id provider_admin)
FINANCE_ROLE=$(role_id finance)
req GET "$API/roles/$CLIENT_ADMIN_ROLE" 200 "get client_admin"
expect "company scope" '.scope == "company" and .is_system'
req GET "$API/roles/does-not-exist" 404 "unknown id"

CUSTOM_NAME="smoke_auditor_$RUN"
req POST "$API/roles" 201 "create custom role" "{\"name\":\"$CUSTOM_NAME\",\"description\":\"Smoke auditor\",\"scope\":\"global\",\"permissions\":[\"audit.read\"]}"
expect "not a system role" '.is_system == false and .active and .permissions == ["audit.read"]'
CUSTOM_ROLE=$(field .id)
req POST "$API/roles" 409 "duplicate name" "{\"name\":\"$CUSTOM_NAME\",\"scope\":\"global\",\"permissions\":[]}"
req POST "$API/roles" 400 "unknown permission" '{"name":"smoke_bad_perm","scope":"global","permissions":["made.up"]}'
req POST "$API/roles" 400 "invalid scope" '{"name":"smoke_bad_scope","scope":"planet","permissions":[]}'
req POST "$API/roles" 400 "invalid name" '{"name":"Bad Name!","scope":"global","permissions":[]}'
req PATCH "$API/roles/$CUSTOM_ROLE" 200 "update description" '{"description":"Reads audit and companies"}'
expect "updated" '.description == "Reads audit and companies"'
req PUT "$API/roles/$CUSTOM_ROLE/permissions" 200 "replace permissions" '{"permissions":["company.read","audit.read"]}'
expect "two permissions" '.permissions | sort == ["audit.read","company.read"]'
req PUT "$API/roles/$CUSTOM_ROLE/permissions" 400 "unknown permission" '{"permissions":["nope.nope"]}'
req PUT "$API/roles/$CUSTOM_ROLE/permissions" 400 "missing permissions" '{}'
req PATCH "$API/roles/$SUPERADMIN_ROLE" 400 "rename system role" '{"name":"root"}'
req PATCH "$API/roles/$SUPERADMIN_ROLE" 400 "deactivate system role" '{"active":false}'
req PUT "$API/roles/$SUPERADMIN_ROLE/permissions" 400 "edit superadmin permissions" '{"permissions":["audit.read"]}'
req PATCH "$API/roles/does-not-exist" 404 "unknown id" '{"description":"x"}'

# ---------------------------------------------------------------------------
section "Role assignments"

req GET "$API/users/$ADMIN_A/roles" 200 "no roles yet"
expect "empty" '.items | length == 0'
req POST "$API/users/$ADMIN_A/roles" 400 "company role without company_id" "{\"role_id\":\"$CLIENT_ADMIN_ROLE\"}"
req POST "$API/users/$ADMIN_A/roles" 400 "global role with company_id" "{\"role_id\":\"$SUPERADMIN_ROLE\",\"company_id\":\"$COMPANY_A\"}"
req POST "$API/users/$ADMIN_A/roles" 400 "unknown role" "{\"role_id\":\"does-not-exist\",\"company_id\":\"$COMPANY_A\"}"
req POST "$API/users/$ADMIN_A/roles" 400 "missing role_id" '{}'
req POST "$API/users/does-not-exist/roles" 404 "unknown user" "{\"role_id\":\"$CLIENT_ADMIN_ROLE\",\"company_id\":\"$COMPANY_A\"}"
req POST "$API/users/$ADMIN_A/roles" 201 "assign client_admin in company A" "{\"role_id\":\"$CLIENT_ADMIN_ROLE\",\"company_id\":\"$COMPANY_A\"}"
expect "active assignment by superadmin" '.active and .assigned_by == $sa and .company_id == $a' --arg sa "$SA_ID" --arg a "$COMPANY_A"
ADMIN_A_ASSIGNMENT=$(field .id)
req POST "$API/users/$ADMIN_A/roles" 409 "same role and scope twice" "{\"role_id\":\"$CLIENT_ADMIN_ROLE\",\"company_id\":\"$COMPANY_A\"}"
req GET "$API/users/$ADMIN_A/roles" 200 "list assignments"
expect "client_admin listed" '(.items | length == 1) and .items[0].role_name == "client_admin"'

# ---------------------------------------------------------------------------
section "Company admin scope (RBAC)"

login "$ADMIN_EMAIL_A" "$PASSWORD" 200 "company admin"
CA=$(field .access_token)
CA_REFRESH=$(field .refresh_token)
TOKEN=$CA

req GET "$API/auth/me" 200 "me"
expect "no global permissions" '.global_permissions | length == 0'
expect "user.manage in company A" '.company_permissions[$a] | index("user.manage")' --arg a "$COMPANY_A"
req GET "$API/companies" 200 "list companies"
expect "sees only company A" '.total == 1 and .items[0].id == $a' --arg a "$COMPANY_A"
req GET "$API/companies/$COMPANY_A" 200 "get own company"
req GET "$API/companies/$COMPANY_B" 403 "get another company"
req POST "$API/companies" 403 "create company" "$(company_body "Nope" "SMK-N-$RUN" "$CO_ID" "$COP_ID" client)"
req PATCH "$API/companies/$COMPANY_A/status" 403 "change company status" '{"status":"suspended"}'
req GET "$API/countries" 200 "read catalog"
req POST "$API/countries" 403 "manage catalog" '{"iso_code":"QX","name":"Nope"}'
req GET "$API/roles" 200 "read roles"
req GET "$API/permissions" 200 "read permissions"
req POST "$API/roles" 403 "create role" '{"name":"smoke_nope","scope":"company","permissions":[]}'
req GET "$API/audit-logs" 403 "read audit"
req GET "$API/users" 200 "list users"
expect "only users of company A" 'all(.items[]; .company_id == $a)' --arg a "$COMPANY_A"
req GET "$API/users/$SA_ID" 403 "read the superadmin"
req GET "$API/users/$ADMIN_A" 200 "read self"
req POST "$API/users" 403 "create user in company B" "$(user_body "b.$RUN@smoke-b.test" "SMK-U2-$RUN" "$COMPANY_B" Bruno)"
req POST "$API/users" 201 "create operator in company A" "$(user_body "op.$RUN@smoke-a.test" "SMK-U3-$RUN" "$COMPANY_A" Oscar)"
OPERATOR=$(field .id)
req PATCH "$API/users/$OPERATOR/status" 200 "activate operator" '{"status":"active"}'
req POST "$API/users/$OPERATOR/roles" 201 "assign client_operator in A" "{\"role_id\":\"$CLIENT_OPERATOR_ROLE\",\"company_id\":\"$COMPANY_A\"}"
OPERATOR_ASSIGNMENT=$(field .id)
req POST "$API/users/$OPERATOR/roles" 403 "assign a global role" "{\"role_id\":\"$FINANCE_ROLE\"}"
req POST "$API/users/$OPERATOR/roles" 403 "assign a role in company B" "{\"role_id\":\"$PROVIDER_ADMIN_ROLE\",\"company_id\":\"$COMPANY_B\"}"
req PATCH "$API/users/$ADMIN_A/status" 400 "change own status" '{"status":"inactive"}'
req DELETE "$API/users/$ADMIN_A/roles/$ADMIN_A_ASSIGNMENT" 400 "revoke own role"
req DELETE "$API/users/$OPERATOR/roles/$OPERATOR_ASSIGNMENT" 204 "revoke operator role"
req DELETE "$API/users/$OPERATOR/roles/$OPERATOR_ASSIGNMENT" 400 "revoke twice"
req DELETE "$API/users/$OPERATOR/roles/does-not-exist" 404 "unknown assignment"
req DELETE "$API/users/$OPERATOR/roles/$ADMIN_A_ASSIGNMENT" 404 "assignment of another user"
req GET "$API/users/$OPERATOR/roles" 200 "operator roles after revocation"
expect "no active roles" '.items | length == 0'

# ---------------------------------------------------------------------------
section "Passwords"

req PUT "$API/users/$ADMIN_A/password" 400 "wrong current password" "{\"current_password\":\"not-my-password\",\"new_password\":\"$NEW_PASSWORD\"}"
req PUT "$API/users/$ADMIN_A/password" 400 "new password too short" "{\"current_password\":\"$PASSWORD\",\"new_password\":\"short\"}"
req PUT "$API/users/$SA_ID/password" 403 "change the superadmin password" "{\"new_password\":\"$NEW_PASSWORD\"}"
req PUT "$API/users/$ADMIN_A/password" 204 "change own password" "{\"current_password\":\"$PASSWORD\",\"new_password\":\"$NEW_PASSWORD\"}"
req POST "$API/auth/refresh" 401 "old sessions revoked after password change" "{\"refresh_token\":\"$CA_REFRESH\"}"
login "$ADMIN_EMAIL_A" "$PASSWORD" 401 "old password"
login "$ADMIN_EMAIL_A" "$NEW_PASSWORD" 200 "new password"
TOKEN=$SA
req PUT "$API/users/$OPERATOR/password" 204 "admin reset without current password" "{\"new_password\":\"$NEW_PASSWORD\"}"
login "op.$RUN@smoke-a.test" "$NEW_PASSWORD" 200 "operator with reset password"

# ---------------------------------------------------------------------------
section "Permissions resolved on every request"

TOKEN=$SA req POST "$API/users/$OPERATOR/roles" 201 "assign custom global role to operator" "{\"role_id\":\"$CUSTOM_ROLE\"}"
login "op.$RUN@smoke-a.test" "$NEW_PASSWORD" 200 "operator"
OP=$(field .access_token)
TOKEN=$OP req GET "$API/audit-logs?page_size=1" 200 "operator reads audit through custom role"
TOKEN=$SA req PATCH "$API/roles/$CUSTOM_ROLE" 200 "deactivate custom role" '{"active":false}'
TOKEN=$OP req GET "$API/audit-logs?page_size=1" 403 "same token loses access at once"
TOKEN=$SA req POST "$API/users/$ADMIN_A/roles" 400 "assign inactive role" "{\"role_id\":\"$CUSTOM_ROLE\"}"
TOKEN=$SA req PATCH "$API/users/$OPERATOR/status" 200 "deactivate operator" '{"status":"inactive"}'
TOKEN=$OP req GET "$API/auth/me" 401 "inactive user token rejected"
login "op.$RUN@smoke-a.test" "$NEW_PASSWORD" 403 "inactive user cannot log in"

# ---------------------------------------------------------------------------
section "Brute force blocking"

TOKEN=$SA
BLOCK_EMAIL="block.$RUN@smoke-a.test"
req POST "$API/users" 201 "create user to block" "$(user_body "$BLOCK_EMAIL" "SMK-U4-$RUN" "$COMPANY_A" Blocky)"
BLOCKED=$(field .id)
req PATCH "$API/users/$BLOCKED/status" 200 "activate" '{"status":"active"}'
for i in $(seq 1 "$MAX_FAILED"); do
	login "$BLOCK_EMAIL" "wrong-password-$i" 401 "failed attempt $i/$MAX_FAILED"
done
login "$BLOCK_EMAIL" "$PASSWORD" 403 "correct password after blocking"
expect "reports blocked" '.error.message | contains("blocked")'
TOKEN=$SA req GET "$API/users/$BLOCKED" 200 "blocked user"
expect "status blocked" '.status == "blocked" and .failed_login_attempts == ($n | tonumber)' --arg n "$MAX_FAILED"
TOKEN=$SA req PATCH "$API/users/$BLOCKED/status" 200 "unblock" '{"status":"active"}'
expect "attempts reset" '.failed_login_attempts == 0'
login "$BLOCK_EMAIL" "$PASSWORD" 200 "login after unblock"

# ---------------------------------------------------------------------------
section "Commerce: public catalog"

COMMERCE=$API/commerce
TOKEN=''
req GET "$COMMERCE/categories" 200 "public categories"
expect "8 seeded categories" '[.items[].code] | contains(["customs_brokerage","ground_transport","warehousing"])'
CUSTOMS_CAT=$(field '.items[] | select(.code=="customs_brokerage") | .id')
GROUND_CAT=$(field '.items[] | select(.code=="ground_transport") | .id')
WAREHOUSE_CAT=$(field '.items[] | select(.code=="warehousing") | .id')
req GET "$COMMERCE/categories?include_inactive=true" 401 "inactive categories need a token"
TOKEN=garbage req GET "$COMMERCE/distributors" 401 "invalid token on a public route"
req GET "$COMMERCE/distributors" 200 "public listing without token"
req GET "$COMMERCE/distributors/does-not-exist" 404 "unknown distributor"

# ---------------------------------------------------------------------------
section "Commerce: actors"

TOKEN=$SA
OPS_EMAIL="ops.$RUN@global360.test"
req POST "$API/users" 201 "create Global 360 reviewer" "$(user_body "$OPS_EMAIL" "SMK-G1-$RUN" "" Olga)"
OPS_ID=$(field .id)
req PATCH "$API/users/$OPS_ID/status" 200 "activate reviewer" '{"status":"active"}'
req POST "$API/users/$OPS_ID/roles" 201 "assign operations" "{\"role_id\":\"$(role_id operations)\"}"
SELLER_EMAIL="seller.$RUN@smoke-b.test"
req POST "$API/users" 201 "create provider admin" "$(user_body "$SELLER_EMAIL" "SMK-P1-$RUN" "$COMPANY_B" Pablo)"
SELLER_ID=$(field .id)
req PATCH "$API/users/$SELLER_ID/status" 200 "activate provider admin" '{"status":"active"}'
req POST "$API/users/$SELLER_ID/roles" 201 "assign provider_admin in company B" "{\"role_id\":\"$PROVIDER_ADMIN_ROLE\",\"company_id\":\"$COMPANY_B\"}"
FOLLOWER_EMAIL="follower.$RUN@smoke-b.test"
req POST "$API/users" 201 "create provider operator" "$(user_body "$FOLLOWER_EMAIL" "SMK-P2-$RUN" "$COMPANY_B" Paula)"
FOLLOWER_ID=$(field .id)
req PATCH "$API/users/$FOLLOWER_ID/status" 200 "activate provider operator" '{"status":"active"}'
req POST "$API/users/$FOLLOWER_ID/roles" 201 "assign provider_operator in company B" "{\"role_id\":\"$(role_id provider_operator)\",\"company_id\":\"$COMPANY_B\"}"
login "$OPS_EMAIL" "$PASSWORD" 200 "reviewer"
OPS=$(field .access_token)
login "$SELLER_EMAIL" "$PASSWORD" 200 "provider admin"
SELLER=$(field .access_token)
login "$FOLLOWER_EMAIL" "$PASSWORD" 200 "provider operator"
FOLLOWER=$(field .access_token)
login "$ADMIN_EMAIL_A" "$NEW_PASSWORD" 200 "client admin"
BUYER=$(field .access_token)

# ---------------------------------------------------------------------------
section "Commerce: distributor profile and services"

PROFILE=$COMMERCE/distributors/$COMPANY_B/profile
SERVICES=$COMMERCE/distributors/$COMPANY_B/services
profile_body() {
	jq -nc --arg cat1 "$CUSTOMS_CAT" --arg cat2 "$GROUND_CAT" --arg co "$CO_ID" --arg cr "$CR_ID" --arg run "$RUN" '{
		display_name: ("Logística Smoke " + $run), summary: "Agencia de aduanas y transporte terrestre Colombia - Costa Rica",
		description: "Nacionalizamos, exportamos y movemos carga entre Colombia y Costa Rica.",
		logo_url: "https://cdn.smoke.test/logo.png", website_url: "https://smoke.test",
		category_ids: [$cat1, $cat2], coverage_country_ids: [$co, $cr],
		contact_email: "ventas@smoke.test", contact_phone: "+57 601 555 0000", whatsapp: "+57 300 555 0000"}'
}
TOKEN=$SELLER
req GET "$PROFILE" 404 "no profile yet"
req PUT "$PROFILE" 400 "missing display_name" '{}'
req PUT "$PROFILE" 400 "unknown category" '{"display_name":"X","category_ids":["does-not-exist"]}'
req PUT "$PROFILE" 400 "invalid website" '{"display_name":"X","website_url":"javascript:alert(1)"}'
req PUT "$PROFILE" 201 "create draft" '{"display_name":"Logística Smoke"}'
expect "draft, not listed" '.status == "draft" and .listed == false and .company_eligible'
req POST "$PROFILE/submit" 400 "submit an incomplete profile"
expect "lists what is missing" '.error.message | contains("summary")'
req PUT "$PROFILE" 200 "complete the profile" "$(profile_body)"
expect "content saved" '(.category_ids | length) == 2 and .whatsapp == "+57 300 555 0000"'
req PUT "$COMMERCE/distributors/$COMPANY_A/profile" 403 "edit another company's profile" "$(profile_body)"
TOKEN=$FOLLOWER req PUT "$PROFILE" 403 "provider operator cannot edit" "$(profile_body)"
TOKEN=$SA req PUT "$COMMERCE/distributors/$COMPANY_A/profile" 400 "client company cannot be a distributor" "$(profile_body)"
TOKEN=$SELLER

service_body() { # name category origin destination
	jq -nc --arg n "$1" --arg c "$2" --arg o "$3" --arg d "$4" \
		'{name:$n, category_id:$c, description:"Servicio de prueba",
		  origin_country_id:(if $o == "" then null else $o end), destination_country_id:(if $d == "" then null else $d end)}'
}
req POST "$SERVICES" 201 "create customs service" "$(service_body "Nacionalización $RUN" "$CUSTOMS_CAT" "$CO_ID" "$CR_ID")"
SERVICE_ID=$(field .id)
req POST "$SERVICES" 409 "duplicate service name" "$(service_body "Nacionalización $RUN" "$CUSTOMS_CAT" "" "")"
req POST "$SERVICES" 400 "unknown origin country" "$(service_body "Otro $RUN" "$CUSTOMS_CAT" "does-not-exist" "")"
req POST "$SERVICES" 400 "missing category" "{\"name\":\"Sin categoria $RUN\"}"
req POST "$SERVICES" 201 "create warehouse service" "$(service_body "Bodega $RUN" "$WAREHOUSE_CAT" "" "$CR_ID")"
INACTIVE_SERVICE=$(field .id)
req PATCH "$SERVICES/$INACTIVE_SERVICE" 200 "deactivate a service" '{"active":false}'
expect "inactive" '.active == false'
req PATCH "$SERVICES/does-not-exist" 404 "unknown service" '{"name":"x"}'
req GET "$SERVICES" 200 "owner lists every service"
expect "includes inactive services" '.items | length == 2'

# ---------------------------------------------------------------------------
section "Commerce: review by Global 360"

TOKEN=$SELLER
req POST "$PROFILE/submit" 200 "submit for review"
expect "pending_review" '.status == "pending_review" and .submitted_at != null'
req PUT "$PROFILE" 400 "locked while under review" "$(profile_body)"
TOKEN='' req GET "$COMMERCE/distributors/$COMPANY_B" 404 "not visible while under review"
req POST "$PROFILE/approve" 403 "seller cannot approve"
TOKEN=$OPS
req GET "$COMMERCE/profiles?status=pending_review" 200 "review queue"
expect "profile queued" 'any(.items[]; .company_id == $b)' --arg b "$COMPANY_B"
req GET "$COMMERCE/profiles?status=bogus" 400 "invalid status filter"
req POST "$PROFILE/reject" 400 "reject without reason" '{}'
req POST "$PROFILE/reject" 200 "reject" '{"reason":"Agregue el NIT en la descripción"}'
expect "rejected with reason" '.status == "rejected" and (.status_reason | contains("NIT"))'
TOKEN=$SELLER req POST "$PROFILE/submit" 200 "resubmit"
expect "reason cleared" '.status_reason == null'
TOKEN=$OPS req POST "$PROFILE/approve" 200 "approve"
expect "published and listed" '.status == "published" and .listed and .published_at != null'
TOKEN=$OPS req POST "$PROFILE/approve" 400 "approve twice"
TOKEN=$SELLER req PUT "$PROFILE" 200 "edit a published profile" "$(profile_body | jq -c '.summary += " (actualizado)"')"
expect "stays published" '.status == "published"'

# ---------------------------------------------------------------------------
section "Commerce: browsing"

TOKEN=''
req GET "$COMMERCE/distributors" 200 "listing"
expect "distributor listed" 'any(.items[]; .company_id == $b)' --arg b "$COMPANY_B"
expect "card resolves references" '.items[] | select(.company_id == $b) | (.categories | length) == 2 and .country.iso_code == "CR"' --arg b "$COMPANY_B"
expect "cards carry no contact data" '[.items[] | has("contact")] | any | not'
req GET "$COMMERCE/distributors?q=aduana" 200 "text search (singular finds plural)"
expect "found" 'any(.items[]; .company_id == $b)' --arg b "$COMPANY_B"
req GET "$COMMERCE/distributors?q=zzqxnomatch" 200 "text search without matches"
expect "empty" '.total == 0'
req GET "$COMMERCE/distributors?category_id=$WAREHOUSE_CAT" 200 "filter by a category it does not have"
expect "not found" 'all(.items[]; .company_id != $b)' --arg b "$COMPANY_B"
req GET "$COMMERCE/distributors?category_id=$CUSTOMS_CAT&country_id=$CR_ID" 200 "filter by category and coverage"
expect "found" 'any(.items[]; .company_id == $b)' --arg b "$COMPANY_B"
req GET "$COMMERCE/distributors?page_size=1" 200 "pagination"
expect "page_size 1" '.page_size == 1 and (.items | length) <= 1'
req GET "$COMMERCE/distributors/$COMPANY_B" 200 "anonymous detail"
expect "no contact data" '.contact == null'
expect "only active services" '(.services | length) == 1 and .services[0].origin_country.iso_code == "CO" and .services[0].category.code == "customs_brokerage"'
TOKEN=$BUYER req GET "$COMMERCE/distributors/$COMPANY_B" 200 "authenticated detail"
expect "contact data disclosed" '.contact.email == "ventas@smoke.test" and .contact.whatsapp == "+57 300 555 0000"'

# ---------------------------------------------------------------------------
section "Commerce: contact requests (leads)"

LEADS=$COMMERCE/distributors/$COMPANY_B/contact-requests
MESSAGE='{"message":"Necesitamos nacionalizar 2 contenedores en Limón."}'
TOKEN='' req POST "$LEADS" 401 "contact without login" "$MESSAGE"
TOKEN=$SELLER req POST "$LEADS" 403 "providers cannot send contact requests" "$MESSAGE"
TOKEN=$BUYER
req POST "$LEADS" 400 "message too short" '{"message":"hola"}'
req POST "$LEADS" 400 "inactive service" "{\"message\":\"Cotización de bodega por favor.\",\"service_id\":\"$INACTIVE_SERVICE\"}"
req POST "$LEADS" 201 "send a contact request" "{\"message\":\"Necesitamos nacionalizar 2 contenedores en Limón.\",\"service_id\":\"$SERVICE_ID\"}"
expect "new, defaults to the requester data" '.status == "new" and .contact_email == $e and (.contact_name | startswith("Ana"))' --arg e "$ADMIN_EMAIL_A"
LEAD_ID=$(field .id)
req POST "$LEADS" 409 "second open request to the same distributor" "$MESSAGE"
req GET "$COMMERCE/contact-requests" 200 "requests sent"
expect "listed" 'any(.items[]; .id == $id and .distributor_id == $b)' --arg id "$LEAD_ID" --arg b "$COMPANY_B"
req GET "$COMMERCE/contact-requests?status=bogus" 400 "invalid status filter"
req GET "$LEADS" 403 "client cannot read the distributor inbox"
TOKEN=$FOLLOWER req GET "$LEADS" 200 "provider operator reads the inbox"
expect "lead received" 'any(.items[]; .id == $id)' --arg id "$LEAD_ID"
TOKEN=$FOLLOWER
req PATCH "$LEADS/$LEAD_ID" 400 "invalid status" '{"status":"won"}'
req PATCH "$LEADS/$LEAD_ID" 400 "nothing to change" '{}'
req PATCH "$LEADS/$LEAD_ID" 200 "mark contacted" '{"status":"contacted","notes":"Llamar el lunes"}'
expect "contacted with notes" '.status == "contacted" and .distributor_notes == "Llamar el lunes"'
req PATCH "$LEADS/$LEAD_ID" 400 "back to new" '{"status":"new"}'
req PATCH "$LEADS/$LEAD_ID" 200 "close" '{"status":"closed"}'
req PATCH "$LEADS/does-not-exist" 404 "unknown request" '{"status":"closed"}'
TOKEN=$BUYER
req GET "$COMMERCE/contact-requests?status=closed" 200 "requester follows the status"
expect "closed, internal notes hidden" 'any(.items[]; .id == $id and .status == "closed" and .distributor_notes == null)' --arg id "$LEAD_ID"
req POST "$LEADS" 201 "new request once the previous one was handled" "$MESSAGE"

# ---------------------------------------------------------------------------
section "Commerce: visibility follows company and profile status"

TOKEN=$SA req PATCH "$API/companies/$COMPANY_B/status" 200 "suspend the provider company" '{"status":"suspended"}'
TOKEN='' req GET "$COMMERCE/distributors/$COMPANY_B" 404 "hidden while the company is suspended"
TOKEN=$BUYER req POST "$LEADS" 404 "cannot contact a hidden distributor" "$MESSAGE"
TOKEN=$SA req PATCH "$API/companies/$COMPANY_B/status" 200 "reactivate the provider company" '{"status":"active"}'
TOKEN='' req GET "$COMMERCE/distributors/$COMPANY_B" 200 "listed again"
TOKEN=$OPS
req POST "$PROFILE/suspend" 400 "suspend without reason" '{}'
req POST "$PROFILE/suspend" 200 "suspend the profile" '{"reason":"Reclamos de clientes"}'
TOKEN='' req GET "$COMMERCE/distributors/$COMPANY_B" 404 "hidden while the profile is suspended"
TOKEN=$OPS req POST "$PROFILE/reinstate" 200 "reinstate"
TOKEN='' req GET "$COMMERCE/distributors/$COMPANY_B" 200 "listed after reinstatement"

# ---------------------------------------------------------------------------
section "Commerce: categories administration"

TOKEN=$OPS
CATEGORY_CODE="smoke_cat_$RUN"
req POST "$COMMERCE/categories" 201 "create category" "{\"code\":\"$CATEGORY_CODE\",\"name\":\"Categoría smoke\"}"
NEW_CATEGORY=$(field .id)
req POST "$COMMERCE/categories" 409 "duplicate code" "{\"code\":\"$CATEGORY_CODE\",\"name\":\"Otra\"}"
req POST "$COMMERCE/categories" 400 "invalid code" '{"code":"Bad Code","name":"X"}'
req PATCH "$COMMERCE/categories/$NEW_CATEGORY" 200 "deactivate category" '{"active":false}'
req PATCH "$COMMERCE/categories/does-not-exist" 404 "unknown category" '{"name":"x"}'
req GET "$COMMERCE/categories?include_inactive=true" 200 "staff sees inactive categories"
expect "inactive included" 'any(.items[]; .id == $id and .active == false)' --arg id "$NEW_CATEGORY"
TOKEN='' req GET "$COMMERCE/categories" 200 "public list"
expect "inactive hidden" 'all(.items[]; .id != $id)' --arg id "$NEW_CATEGORY"
TOKEN=$SELLER req POST "$COMMERCE/categories" 403 "provider cannot manage categories" '{"code":"nope_nope","name":"X"}'
TOKEN=$SELLER req GET "$COMMERCE/profiles" 403 "provider cannot read the review queue"
TOKEN=$SA req GET "$API/audit-logs?entity=distributor_profile&entity_id=$COMPANY_B&action=approve" 200 "approval audit trail"
expect "before/after recorded" 'any(.items[]; .old_values.status == "pending_review" and .new_values.status == "published")'
TOKEN=$SA req GET "$API/audit-logs?entity=distributor_profile&entity_id=$COMPANY_B&action=reject" 200 "rejection audit trail"
expect "rejection recorded" '.total >= 1'

# ---------------------------------------------------------------------------
section "Audit log"

TOKEN=$SA
req GET "$API/audit-logs" 200 "list"
expect "has entries" '.total > 0'
req GET "$API/audit-logs?entity=company&entity_id=$COMPANY_A&action=update&page_size=100" 200 "company A updates"
expect "status change with before/after" 'any(.items[]; .old_values.status == "pending_validation" and .new_values.status == "active")'
expect "actor is the superadmin" 'all(.items[]; .user_id == $sa)' --arg sa "$SA_ID"
req GET "$API/audit-logs?action=login&page_size=100" 200 "filter by action"
expect "only logins" '.total > 0 and all(.items[]; .action == "login")'
req GET "$API/audit-logs?user_id=$ADMIN_A&page_size=100" 200 "filter by user"
expect "only that actor" 'all(.items[]; .user_id == $u)' --arg u "$ADMIN_A"
req GET "$API/audit-logs?entity=role_assignment&page_size=100" 200 "role assignment trail"
expect "includes revocation" 'any(.items[]; .entity_id == $id and .new_values.active == false)' --arg id "$OPERATOR_ASSIGNMENT"
FROM=$(date -u -d '-1 hour' +%Y-%m-%dT%H:%M:%SZ)
TO=$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)
req GET "$API/audit-logs?from=$FROM&to=$TO&page_size=1" 200 "date range"
req GET "$API/audit-logs?from=yesterday" 400 "invalid date"
req GET "$API/audit-logs?action=bogus" 400 "invalid action"

LEAK=0
PAGE=1
while :; do
	req GET "$API/audit-logs?page=$PAGE&page_size=100" 200 "scan page $PAGE for secrets" >/dev/null
	grep -q 'password_hash\|\$2a\$' <<<"$BODY" && LEAK=1
	(($(field '.items | length') < 100)) && break
	PAGE=$((PAGE + 1))
done
if ((LEAK == 0)); then record ok "no password hash in any audit entry"; else record fail "no password hash in any audit entry" "found"; fi

# ---------------------------------------------------------------------------
section "Logout"

TOKEN='' req POST "$API/auth/logout" 401 "without access token" "{\"refresh_token\":\"$SA_REFRESH\"}"
req POST "$API/auth/logout" 400 "without refresh_token" '{}'
req POST "$API/auth/logout" 204 "superadmin" "{\"refresh_token\":\"$SA_REFRESH\"}"
req POST "$API/auth/refresh" 401 "refresh token after logout" "{\"refresh_token\":\"$SA_REFRESH\"}"

# ---------------------------------------------------------------------------
printf '\n%sResult: %d passed, %d failed%s\n' "$BOLD" "$PASS" "$FAIL" "$RESET"
if ((FAIL > 0)); then
	printf '\nFailures:\n'
	printf '  - %s\n' "${FAILURES[@]}"
	exit 1
fi
