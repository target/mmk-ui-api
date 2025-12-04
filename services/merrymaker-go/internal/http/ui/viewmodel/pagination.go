package viewmodel

// Pagination contains pagination metadata for list views.
type Pagination struct {
	Page       int
	PageSize   int
	HasPrev    bool
	HasNext    bool
	StartIndex int
	EndIndex   int
	TotalCount int
	PrevURL    string
	NextURL    string
}
