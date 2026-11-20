package odm

// SearchHit is a search result with its relevance score (higher is better).
type SearchHit[T Model] struct {
	Score float64 `bson:"score"`
	Doc   T       `bson:"doc"`
}
