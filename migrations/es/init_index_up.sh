if [[ "$#" -lt "2" || "$#" -gt "4" ]]
then
    echo "Usage: $0 <es_host_with_port> <index_name> [<username>] [<password>]"
    exit 1
fi

HOST="$1"
INDEX="$2"
USERNAME="$3"

AUTH="$USERNAME"
if [ "$#" == "4" ]; then
    AUTH="$AUTH:$4"
fi

echo "Checking if index exists..."

STATUSCODE=$(curl -I -s -o /dev/null -w "%{http_code}" -u $AUTH "$HOST"/"$INDEX")

if [ $STATUSCODE == "200" ]; then
    echo "Index already exists"
    exit 0
fi

echo "Creating index..."

curl -XPUT -s -H 'Content-Type: application/json' -u $AUTH "$HOST"/"$INDEX" -d '{
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
