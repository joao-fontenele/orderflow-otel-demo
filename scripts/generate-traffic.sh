#!/bin/bash
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
ORDER_COUNT="${ORDER_COUNT:-10}"

echo "Generating traffic against $GATEWAY_URL"
echo "Creating $ORDER_COUNT orders"
echo ""

for i in $(seq 1 "$ORDER_COUNT"); do
  item_num=$((RANDOM % 10 + 1))
  item_id=$(printf "ITEM-%03d" "$item_num")
  quantity=$((RANDOM % 5 + 1))
  price=$((RANDOM % 10000 + 1000))

  if [ $((RANDOM % 10)) -eq 0 ]; then
    item_id="INVALID-ITEM"
  fi

  items=$(cat <<EOF
[{"item_id": "$item_id", "quantity": $quantity, "price": $price}]
EOF
)

  echo "Creating order $i (item: $item_id, qty: $quantity)..."
  response=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/orders" \
    -H "Content-Type: application/json" \
    -d "{\"customer_id\": \"customer-$i\", \"items\": $items}" 2>&1 || true)

  http_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')

  if [ "$http_code" = "201" ]; then
    echo "  -> Created: $body"
  else
    echo "  -> Failed (HTTP $http_code): $body"
  fi
done

echo ""
echo "Checking inventory levels..."

for id in ITEM-001 ITEM-002 ITEM-003; do
  echo "Inventory for $id:"
  curl -s "$GATEWAY_URL/inventory/stock/$id" 2>&1 || echo "  -> Failed to fetch"
  echo ""
done

echo ""
echo "Listing recent orders..."
curl -s "$GATEWAY_URL/orders" | head -c 500
echo ""
echo ""
echo "Traffic generation complete!"
