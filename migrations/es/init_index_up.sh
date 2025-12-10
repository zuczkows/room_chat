HOST="$1"
INDEX="$2"

echo "Checking if index exists..."

STATUSCODE=$(curl -I -s -o /dev/null -w "%{http_code}" "$HOST"/"$INDEX")

if [ $STATUSCODE == "200" ]; then
    echo "Index already exists"
    exit 0
fi

echo "Creating index..."

curl -XPUT -s -H 'Content-Type: application/json' "$HOST"/"$INDEX" -d '{
    "settings": {
        "number_of_shards": 2,
        "number_of_replicas": 0
    },
    "mappings": {
        "properties": {
            "message_id": { "type": "keyword" },
            "channel_id": { "type": "keyword" },
            "author_id":  { "type": "keyword" },
            "content":    { "type": "text" },
            "created_at": { "type": "date" }
        }
    }
}'

echo ""
echo "Index created"
