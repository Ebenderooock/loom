package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const imdbGraphQLEndpoint = "https://api.graphql.imdb.com/"

const imdbTitleFields = `titleListItemSearch(first: $first, after: $after, sort: {by: LIST_ORDER, order: ASC}) {
	edges { title { id titleText { text } releaseYear { year } titleType { id } } }
	pageInfo { hasNextPage endCursor }
}`

var (
	imdbListID    = regexp.MustCompile(`^ls\d+$`)
	imdbUserID    = regexp.MustCompile(`^ur\d+$`)
	imdbProfileID = regexp.MustCompile(`^p\.[A-Za-z0-9]+$`)
	imdbTitleID   = regexp.MustCompile(`^tt\d+$`)
)

// IMDbProvider fetches public IMDb lists and watchlists.
type IMDbProvider struct {
	client *http.Client
}

// NewIMDbList returns a provider for public IMDb lists.
func NewIMDbList() *IMDbProvider {
	return &IMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

// NewIMDbWatchlist returns a provider for public IMDb watchlists.
func NewIMDbWatchlist() *IMDbProvider {
	return &IMDbProvider{client: &http.Client{Timeout: 30 * time.Second}}
}

func (p *IMDbProvider) Fetch(ctx context.Context, cfg ProviderConfig) ([]Item, error) {
	input := strings.TrimSpace(cfg.URL)
	if input == "" {
		return nil, fmt.Errorf("imdb: URL required")
	}

	root, id, err := imdbSource(input)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return p.fetchRSS(ctx, input)
	}
	if imdbProfileID.MatchString(id) {
		data, err := p.query(ctx, `query($id: ID!) { userProfile(input: {profileId: $id}) { userId } }`, map[string]any{"id": id})
		if err != nil {
			return nil, err
		}
		if data.UserProfile == nil || !imdbUserID.MatchString(data.UserProfile.UserID) {
			return nil, fmt.Errorf("imdb: profile not found; check the watchlist URL and make it public")
		}
		id = data.UserProfile.UserID
	}

	field := "list(id: $id)"
	if root == "predefinedList" {
		field = "predefinedList(classType: WATCH_LIST, userId: $id)"
	}
	query := "query($id: ID!, $first: Int!, $after: String) { " + field + " { " + imdbTitleFields + " } }"
	variables := map[string]any{"id": id, "first": 250, "after": nil}
	items := make([]Item, 0)
	seenIDs := make(map[string]bool)
	seenCursors := make(map[string]bool)
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		data, err := p.query(ctx, query, variables)
		if err != nil {
			return nil, err
		}
		list := data.List
		if root == "predefinedList" {
			list = data.PredefinedList
		}
		if list == nil {
			return nil, fmt.Errorf("imdb: list not found or private; check the URL and make the list public")
		}
		page := list.Items
		if page == nil || page.Edges == nil || page.PageInfo == nil {
			return nil, fmt.Errorf("imdb: incomplete list response")
		}
		for _, edge := range page.Edges {
			title := edge.Title
			if title == nil || !imdbTitleID.MatchString(title.ID) {
				return nil, fmt.Errorf("imdb: list contains an invalid title")
			}
			if seenIDs[title.ID] {
				continue
			}
			seenIDs[title.ID] = true
			item := Item{ExternalID: title.ID, IMDbID: title.ID, Title: title.TitleText.Text}
			if item.Title == "" {
				item.Title = title.ID
			}
			if title.ReleaseYear != nil {
				item.Year = title.ReleaseYear.Year
			}
			switch title.TitleType.ID {
			case "tvSeries", "tvMiniSeries":
				item.MediaType = "series"
			case "movie", "tvMovie", "short", "tvShort", "video":
				item.MediaType = "movie"
			}
			items = append(items, item)
		}
		if !page.PageInfo.HasNextPage {
			return items, nil
		}
		cursor := page.PageInfo.EndCursor
		if cursor == "" || seenCursors[cursor] || len(page.Edges) == 0 {
			return nil, fmt.Errorf("imdb: invalid pagination cursor")
		}
		seenCursors[cursor] = true
		variables["after"] = cursor
	}
	return nil, fmt.Errorf("imdb: list exceeded 1000 pages without finishing")
}

func imdbSource(input string) (string, string, error) {
	if imdbListID.MatchString(input) {
		return "list", input, nil
	}
	if imdbUserID.MatchString(input) || imdbProfileID.MatchString(input) {
		return "predefinedList", input, nil
	}
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", "", fmt.Errorf("imdb: expected an IMDb list or public watchlist URL")
	}
	switch strings.ToLower(u.Hostname()) {
	case "imdb.com", "www.imdb.com", "m.imdb.com", "rss.imdb.com":
	default:
		// Keep support for explicitly configured RSS exports.
		return "", "", nil
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "list" && imdbListID.MatchString(parts[i+1]) {
			return "list", parts[i+1], nil
		}
		if parts[i] == "user" && i+2 < len(parts) && parts[i+2] == "watchlist" &&
			(imdbUserID.MatchString(parts[i+1]) || imdbProfileID.MatchString(parts[i+1])) {
			return "predefinedList", parts[i+1], nil
		}
	}
	return "", "", fmt.Errorf("imdb: expected an IMDb list or public watchlist URL")
}

func (p *IMDbProvider) query(ctx context.Context, query string, variables map[string]any) (*imdbData, error) {
	body, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{query, variables})
	if err != nil {
		return nil, fmt.Errorf("imdb: encode query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, imdbGraphQLEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("imdb: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-imdb-client-name", "imdb-web-next")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imdb: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdb: unexpected status %d; check that the list is public", resp.StatusCode)
	}
	const maxBody = 10 << 20
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("imdb: read response: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("imdb: response exceeds 10 MiB")
	}
	var result struct {
		Data   *imdbData `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("imdb: parse response: %w", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("imdb: %s; check the URL and ensure the list is public", result.Errors[0].Message)
	}
	if result.Data == nil {
		return nil, fmt.Errorf("imdb: missing response data")
	}
	return result.Data, nil
}

type imdbData struct {
	List           *imdbList `json:"list"`
	PredefinedList *imdbList `json:"predefinedList"`
	UserProfile    *struct {
		UserID string `json:"userId"`
	} `json:"userProfile"`
}

type imdbList struct {
	Items *struct {
		Edges []struct {
			Title *struct {
				ID        string `json:"id"`
				TitleText struct {
					Text string `json:"text"`
				} `json:"titleText"`
				ReleaseYear *struct {
					Year int `json:"year"`
				} `json:"releaseYear"`
				TitleType struct {
					ID string `json:"id"`
				} `json:"titleType"`
			} `json:"title"`
		} `json:"edges"`
		PageInfo *struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"titleListItemSearch"`
}

func (p *IMDbProvider) fetchRSS(ctx context.Context, feedURL string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imdb: build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imdb: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdb: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("imdb: read body: %w", err)
	}

	var rss imdbRSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, fmt.Errorf("imdb: parse RSS: %w", err)
	}

	yearRe := regexp.MustCompile(`\((\d{4})\)`)
	imdbIDRe := regexp.MustCompile(`tt\d+`)

	var items []Item
	for _, entry := range rss.Channel.Items {
		title := entry.Title
		year := 0
		if m := yearRe.FindStringSubmatch(title); len(m) == 2 {
			year, _ = strconv.Atoi(m[1])
			title = strings.TrimSpace(yearRe.ReplaceAllString(title, ""))
		}
		imdbID := ""
		if m := imdbIDRe.FindString(entry.Link); m != "" {
			imdbID = m
		}

		items = append(items, Item{
			ExternalID: imdbID,
			Title:      title,
			Year:       year,
			IMDbID:     imdbID,
		})
	}
	return items, nil
}

type imdbRSS struct {
	XMLName xml.Name       `xml:"rss"`
	Channel imdbRSSChannel `xml:"channel"`
}

type imdbRSSChannel struct {
	Items []imdbRSSItem `xml:"item"`
}

type imdbRSSItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}
