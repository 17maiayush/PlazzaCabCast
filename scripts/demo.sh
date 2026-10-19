#!/usr/bin/env bash
# Walks the API end to end against a running server: `make run` in one terminal, `make demo` in another.
# Every edge case from the assignment is in here: no driver in radius, minimum fare, the free
# hatchback-to-sedan upgrade, an invalid coupon, plus idempotent booking, surge and cancellation.
set -euo pipefail

BASE=${BASE:-http://localhost:8080}
MG_ROAD='{"lat":12.9752,"lng":77.6058}'
INDIRANAGAR='{"lat":12.9719,"lng":77.6412}'   # ~3.9 km from MG Road: metered fare is below the minimum
ELECTRONIC_CITY='{"lat":12.8456,"lng":77.6603}' # ~15.6 km from MG Road
WHITEFIELD='{"lat":12.9698,"lng":77.7500}'      # no driver within 5 km

say()  { printf '\n\033[1m%s\033[0m\n' "$*"; }
show() { if command -v jq >/dev/null 2>&1; then jq . <<<"$1"; else echo "$1"; fi; }
id_of() { sed -E 's/^\{"id":"([^"]+)".*/\1/' <<<"$1"; }

# req METHOD PATH [JSON-BODY] [HEADER]...  -> prints status + body, leaves the body in $LAST
req() {
  local method=$1 path=$2 body=${3:-}
  shift $(( $# < 3 ? $# : 3 ))
  local args=(-s -w '\n%{http_code}' -X "$method" "$BASE$path" -H 'Content-Type: application/json')
  [ -n "$body" ] && args+=(-d "$body")
  for header in "$@"; do args+=(-H "$header"); done
  local out; out=$(curl "${args[@]}")
  local code=${out##*$'\n'}
  LAST=${out%$'\n'*}
  printf '%s %s -> %s\n' "$method" "$path" "$code"
  if [ -n "$LAST" ]; then show "$LAST"; fi
}

say "0. health"
req GET /healthz

say "1. register a user and three drivers (two near MG Road, one 13 km away)"
req POST /users '{"name":"Asha"}';                                                      ASHA=$(id_of "$LAST")
req POST /drivers '{"name":"Ravi","car_type":"hatchback","location":{"lat":12.9780,"lng":77.6100},"rating":4.6}'; RAVI=$(id_of "$LAST")
req POST /drivers '{"name":"Meena","car_type":"sedan","location":{"lat":12.9700,"lng":77.6000},"rating":4.9}';   MEENA=$(id_of "$LAST")
req POST /drivers '{"name":"Arun","car_type":"hatchback","location":{"lat":13.0500,"lng":77.7000}}';            ARUN=$(id_of "$LAST")

say "2. coupons: 20% off and a flat Rs 30 off"
req POST /coupons '{"code":"SAVE20","kind":"percent","percent":20}'
req POST /coupons '{"code":"FLAT30","kind":"flat","amount_paise":3000}'

say "3. edge: no driver within 5 km of Whitefield -> 409"
req POST /rides "{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$WHITEFIELD}"

say "4. edge: invalid coupon -> 400, and no driver gets reserved"
req POST /rides "{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$MG_ROAD,\"coupon_code\":\"NOPE\"}"

say "5. book a hatchback from MG Road -> nearest hatchback (Ravi)"
req POST /rides "{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$MG_ROAD}";          RIDE1=$(id_of "$LAST")

say "6. edge: book another hatchback while Ravi is busy -> upgraded to Meena's sedan, upgraded=true"
req POST /rides "{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$MG_ROAD}";          RIDE2=$(id_of "$LAST")

say "7. edge: end ride 1 after ~3.9 km -> metered Rs 34.xx, so the Rs 50 minimum applies"
req POST "/rides/$RIDE1/end" "{\"drop\":$INDIRANAGAR}"

say "8. end the upgraded ride after ~15.6 km -> charged the HATCHBACK tariff (~Rs 97), not the sedan one (~Rs 150)"
req POST "/rides/$RIDE2/end" "{\"drop\":$ELECTRONIC_CITY}"

say "9. update a cab's location: Meena drives back to MG Road"
req PUT "/drivers/$MEENA/location" '{"lat":12.9700,"lng":77.6000}'

say "10. ride with coupon SAVE20: Ravi is 3.9 km away at Indiranagar, still inside the radius"
req POST /rides "{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$MG_ROAD,\"coupon_code\":\"save20\"}"; RIDE3=$(id_of "$LAST")
req POST "/rides/$RIDE3/end" "{\"drop\":$ELECTRONIC_CITY}"
req PUT "/drivers/$RAVI/location" '{"lat":12.9780,"lng":77.6100}'

say "11. idempotent booking: same Idempotency-Key twice -> one ride, second response is a replay"
BODY="{\"user_id\":\"$ASHA\",\"car_type\":\"hatchback\",\"pickup\":$MG_ROAD}"
req POST /rides "$BODY" 'Idempotency-Key: demo-key-1';                                              RIDE4=$(id_of "$LAST")
curl -s -D - -o /dev/null -X POST "$BASE/rides" -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-key-1' -d "$BODY" | grep -i -E 'HTTP/|Idempotent-Replayed'
req POST /rides "${BODY/77.6058/77.6059}" 'Idempotency-Key: demo-key-1'

say "12. surge: one more driver, then bookings until demand outruns supply (2 ongoing vs 1 free -> 1.25x)"
req POST /drivers '{"name":"Divya","car_type":"hatchback","location":{"lat":12.9740,"lng":77.6020}}'
req POST /rides "$BODY";                                                                              RIDE5=$(id_of "$LAST")
req POST /rides "$BODY";                                                                              RIDE6=$(id_of "$LAST")

say "13. cancel the surged ride inside the 3-minute grace window -> no fee, driver released"
req POST "/rides/$RIDE6/cancel"

say "14. history: everything for Asha, then only ongoing, then Ravi's completed rides"
req GET "/users/$ASHA/rides"
req GET "/users/$ASHA/rides?status=ongoing"
req GET "/drivers/$RAVI/rides?status=completed"

say "15. delete a coupon, then try again -> 404"
req DELETE /coupons/SAVE20
req DELETE /coupons/SAVE20

say "done: rides $RIDE4 and $RIDE5 are still ongoing; Arun ($ARUN) never got a ride, he is out of radius"
