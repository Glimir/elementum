package tmdb

import (
	"fmt"
	"path"
	"sync"
	"time"
	"strconv"
	"strings"
	"math/rand"

	"github.com/jmcvetta/napping"
	"github.com/scakemyer/quasar/cache"
	"github.com/scakemyer/quasar/config"
	"github.com/scakemyer/quasar/xbmc"
)

func GetShow(showId int, language string) *Show {
	var show *Show
	cacheStore := cache.NewFileStore(path.Join(config.Get().ProfilePath, "cache"))
	key := fmt.Sprintf("com.tmdb.show.%d.%s", showId, language)
	if err := cacheStore.Get(key, &show); err != nil {
		rateLimiter.Call(func() {
			urlValues := napping.Params{
				"api_key": apiKey,
				"append_to_response": "credits,images,alternative_titles,translations,external_ids",
				"language": language,
			}.AsUrlValues()
			resp, err := napping.Get(
				tmdbEndpoint + "tv/" + strconv.Itoa(showId),
				&urlValues,
				&show,
				nil,
			)
			if err != nil {
				log.Error(err.Error())
				xbmc.Notify("Quasar", "GetShow failed, check your logs.", config.AddonIcon())
			} else if resp.Status() != 200 {
				message := fmt.Sprintf("GetShow bad status: %d", resp.Status())
				log.Error(message)
				xbmc.Notify("Quasar", message, config.AddonIcon())
			}
		})
		if show != nil {
			cacheStore.Set(key, show, cacheTime)
		}
	}
	if show == nil {
		return nil
	}
	switch t := show.RawPopularity.(type) {
	case string:
		if popularity, err := strconv.ParseFloat(t, 64); err == nil {
			show.Popularity = popularity
		}
	case float64:
		show.Popularity = t
	}
	return show
}

func GetShows(showIds []int, language string) Shows {
	var wg sync.WaitGroup
	shows := make(Shows, len(showIds))
	wg.Add(len(showIds))
	for i, showId := range showIds {
		go func(i int, showId int) {
			defer wg.Done()
			shows[i] = GetShow(showId, language)
		}(i, showId)
	}
	wg.Wait()
	return shows
}

func SearchShows(query string, language string, page int) Shows {
	var results EntityList
	rateLimiter.Call(func() {
		urlValues := napping.Params{
			"api_key": apiKey,
			"query": query,
			"page": strconv.Itoa(StartPage + page),
		}.AsUrlValues()
		resp, err := napping.Get(
			tmdbEndpoint + "search/tv",
			&urlValues,
			&results,
			nil,
		)
		if err != nil {
			log.Error(err.Error())
			xbmc.Notify("Quasar", "SearchShows failed, check your logs.", config.AddonIcon())
		} else if resp.Status() != 200 {
			message := fmt.Sprintf("SearchShows bad status: %d", resp.Status())
			log.Error(message)
			xbmc.Notify("Quasar", message, config.AddonIcon())
		}
	})
	tmdbIds := make([]int, 0, len(results.Results))
	for _, entity := range results.Results {
		tmdbIds = append(tmdbIds, entity.Id)
	}
	return GetShows(tmdbIds, language)
}

func ListShowsComplete(endpoint string, params napping.Params, page int) Shows {
	resultsPerPage := config.Get().ResultsPerPage
	maxPages := MaxPages
	if page >= 0 {
		maxPages = 1
	}
	shows := make(Shows, maxPages * resultsPerPage)

	params["api_key"] = apiKey

	wg := sync.WaitGroup{}
	for i := 0; i < maxPages; i++ {
		wg.Add(1)
		currentpage := i
		startIndex := i * resultsPerPage
		if page >= 0 {
			currentpage = page
		}
		go func(page int) {
			defer wg.Done()
			var tmp *EntityList
			tmpParams := napping.Params{
				"page": strconv.Itoa(StartPage + page),
			}
			for k, v := range params {
				tmpParams[k] = v
			}
			urlValues := tmpParams.AsUrlValues()
			rateLimiter.Call(func() {
				resp, err := napping.Get(
					tmdbEndpoint + endpoint,
					&urlValues,
					&tmp,
					nil,
				)
				if err != nil {
					log.Error(err.Error())
					xbmc.Notify("Quasar", "ListShows failed, check your logs.", config.AddonIcon())
				} else if resp.Status() != 200 {
					message := fmt.Sprintf("ListShows bad status: %d", resp.Status())
					xbmc.Notify("Quasar", message, config.AddonIcon())
				}
			})
			if tmp != nil {
				for i, entity := range tmp.Results {
					shows[startIndex + i] = GetShow(entity.Id, params["language"])
				}
			}
		}(currentpage)
	}
	wg.Wait()

	return shows
}

func PopularShowsComplete(genre string, language string, page int) Shows {
	var p napping.Params
	if genre == "" {
		p = napping.Params{
			"language":           language,
			"sort_by":            "popularity.desc",
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":           language,
			"sort_by":            "popularity.desc",
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":        genre,
		}
	}
	return ListShowsComplete("discover/tv", p, page)
}

func RecentShowsComplete(genre string, language string, page int) Shows {
	var p napping.Params
	if genre == "" {
		p = napping.Params{
			"language":           language,
			"sort_by":            "first_air_date.desc",
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":           language,
			"sort_by":            "first_air_date.desc",
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":        genre,
		}
	}
	return ListShowsComplete("discover/tv", p, page)
}

func RecentEpisodesComplete(genre string, language string, page int) Shows {
	var p napping.Params

	if genre == "" {
		p = napping.Params{
			"language":           language,
			"air_date.gte": time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"),
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
		}
	} else {
		p = napping.Params{
			"language":           language,
			"air_date.gte": time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"),
			"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
			"with_genres":        genre,
		}
	}
	return ListShowsComplete("discover/tv", p, page)
}

func TopRatedShowsComplete(genre string, language string, page int) Shows {
	return ListShowsComplete("tv/top_rated", napping.Params{"language": language}, page)
}

func MostVotedShowsComplete(genre string, language string, page int) Movies {
	return ListMoviesComplete("discover/tv", napping.Params{
		"language":           language,
		"sort_by":            "vote_count.desc",
		"first_air_date.lte": time.Now().UTC().Format("2006-01-02"),
		"with_genres":        genre,
	}, page)
}

func GetTVGenres(language string) []*Genre {
	genres := GenreList{}
	rateLimiter.Call(func() {
		urlValues := napping.Params{
			"api_key": apiKey,
			"language": language,
		}.AsUrlValues()
		resp, err := napping.Get(
			tmdbEndpoint + "genre/tv/list",
			&urlValues,
			&genres,
			nil,
		)
		if err != nil {
			log.Error(err.Error())
			xbmc.Notify("Quasar", "GetTVGenres failed, check your logs.", config.AddonIcon())
		} else if resp.Status() != 200 {
			message := fmt.Sprintf("GetTVGenres bad status: %d", resp.Status())
			log.Error(message)
			xbmc.Notify("Quasar", message, config.AddonIcon())
		}
	})
	return genres.Genres
}

func (show *Show) ToListItem() *xbmc.ListItem {
	year, _ := strconv.Atoi(strings.Split(show.FirstAirDate, "-")[0])

	name := show.Name
	if config.Get().UseOriginalTitle && show.OriginalName != "" {
		name = show.OriginalName
	}

	item := &xbmc.ListItem{
		Label: name,
		Info: &xbmc.ListItemInfo{
			Year:          year,
			Count:         rand.Int(),
			Title:         name,
			OriginalTitle: show.OriginalName,
			Plot:          show.Overview,
			PlotOutline:   show.Overview,
			Code:          show.ExternalIDs.IMDBId,
			Date:          show.FirstAirDate,
			Votes:         strconv.Itoa(show.VoteCount),
			Rating:        show.VoteAverage,
			TVShowTitle:   show.OriginalName,
			Premiered:     show.FirstAirDate,
		},
		Art: &xbmc.ListItemArt{
			FanArt: ImageURL(show.BackdropPath, "w1280"),
			Poster: ImageURL(show.PosterPath, "w500"),
		},
	}
	item.Thumbnail = item.Art.Poster
	item.Art.Thumbnail = item.Art.Poster

	if show.InProduction {
		item.Info.Status = "Continuing"
	} else {
		item.Info.Status = "Discontinued"
	}

	genres := make([]string, 0, len(show.Genres))
	for _, genre := range show.Genres {
		genres = append(genres, genre.Name)
	}
	item.Info.Genre = strings.Join(genres, " / ")

	for _, company := range show.ProductionCompanies {
		item.Info.Studio = company.Name
		break
	}
	if show.Credits != nil {
		item.Info.CastAndRole = make([][]string, 0)
		for _, cast := range show.Credits.Cast {
			item.Info.CastAndRole = append(item.Info.CastAndRole, []string{cast.Name, cast.Character})
		}
		directors := make([]string, 0)
		writers := make([]string, 0)
		for _, crew := range show.Credits.Crew {
			switch crew.Job {
			case "Director":
				directors = append(directors, crew.Name)
			case "Writer":
				writers = append(writers, crew.Name)
			}
		}
		item.Info.Director = strings.Join(directors, " / ")
		item.Info.Writer = strings.Join(writers, " / ")
	}
	return item
}
