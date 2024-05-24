package typeset

import (
	"github.com/jeffwilliams/anvil/internal/cache"
)

var layoutCaches = cache.New[layoutCacheKey, cache.Cache[string, []Line]](10)

func layoutCacheForConstraints(constraints Constraints) cache.Cache[string, []Line] {
	k := layoutCacheKey{
		constraints.FontSize,
		constraints.FontFaceId,
		constraints.WrapWidth,
		constraints.TabStopInterval,
	}

	entry := layoutCaches.Get(k)
	var cache cache.Cache[string, []Line]
	if entry == nil {
		cache = addNewLayoutCache(k)
	} else {
		cache = entry.Val
	}

	return cache
}

func addNewLayoutCache(k layoutCacheKey) cache.Cache[string, []Line] {
	cache := cache.New[string, []Line](200)
	layoutCaches.Set(k, cache)
	return cache
}

type layoutCacheKey struct {
	FontSize        int
	FaceId          string
	WrapWidth       int
	TabStopInterval int
}
