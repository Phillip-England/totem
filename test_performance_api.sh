#!/bin/bash

BASE_URL="http://localhost:8080"
LOCATION_ID=1

echo "=== Testing Performance API ==="
echo ""

# Default: Last 90 days
echo "1. Default (last 90 days):"
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance" | jq '.summary'
echo ""

# Last 30 days
echo "2. Last 30 days:"
END_DATE=$(date +%Y-%m-%d)
START_DATE=$(date -v-30d +%Y-%m-%d)
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.summary'
echo ""

# Last 7 days
echo "3. Last 7 days:"
START_DATE=$(date -v-7d +%Y-%m-%d)
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.summary'
echo ""

# Current month
echo "4. Current month:"
START_DATE=$(date +%Y-%m-01)
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.summary'
echo ""

# Previous month (December 2025)
echo "5. Previous month (December 2025):"
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=2025-12-01&end=2025-12-31" | jq '.summary'
echo ""

# Year to date
echo "6. Year to date:"
START_DATE=$(date +%Y-01-01)
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.summary'
echo ""

# Full response with daily records (last 7 days)
echo "7. Full response with daily records (last 7 days):"
START_DATE=$(date -v-7d +%Y-%m-%d)
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.'
echo ""

# Just the records array (last 7 days)
echo "8. Just daily records (last 7 days):"
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance?start=$START_DATE&end=$END_DATE" | jq '.records'
echo ""

# Days with actual data only (last 90 days)
echo "9. Days with sales or labor data only (last 90 days):"
curl -s "$BASE_URL/api/locations/$LOCATION_ID/performance" | jq '[.records[] | select(.HasSales or .HasLabor)]'
echo ""

# Different location (change LOCATION_ID as needed)
echo "10. Location 2 - Last 30 days:"
START_DATE=$(date -v-30d +%Y-%m-%d)
END_DATE=$(date +%Y-%m-%d)
curl -s "$BASE_URL/api/locations/2/performance?start=$START_DATE&end=$END_DATE" | jq '.summary'
