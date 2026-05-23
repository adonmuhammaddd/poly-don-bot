package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// RESTClient talks to gamma-api.polymarket.com with a token-bucket rate limiter.
type RESTClient struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
	logger     *slog.Logger
}

func NewRESTClient(baseURL string, requestsPerMinute int, logger *slog.Logger) *RESTClient {
	if requestsPerMinute <= 0 {
		requestsPerMinute = 80
	}
	return &RESTClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(float64(requestsPerMinute)/60.0), requestsPerMinute),
		logger:     logger.With(slog.String("component", "polymarket_rest")),
	}
}

// FindCurrentMarket returns the active market whose slug starts with slugPrefix
// and whose endDate is the soonest in the future. Returns (nil, nil) when none.
func (r *RESTClient) FindCurrentMarket(ctx context.Context, slugPrefix string) (*Market, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	q := url.Values{}
	q.Set("closed", "false")
	q.Set("active", "true")
	q.Set("end_date_min", time.Now().UTC().Format(time.RFC3339))
	q.Set("order", "endDate")
	q.Set("ascending", "true")
	q.Set("limit", "50")
	endpoint := r.baseURL + "/events?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var events []gammaEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	for _, ev := range events {
		if !strings.HasPrefix(ev.Slug, slugPrefix) {
			continue
		}
		if len(ev.Markets) == 0 {
			continue
		}
		m := &ev.Markets[0]
		yes, no, err := m.parseTokenIDs()
		if err != nil {
			r.logger.Warn("skipping market with bad token ids",
				slog.String("slug", ev.Slug),
				slog.Any("error", err),
			)
			continue
		}
		endDate, err := time.Parse(time.RFC3339, ev.EndDate)
		if err != nil {
			continue
		}
		return &Market{
			Slug:        ev.Slug,
			Question:    m.Question,
			ConditionID: m.ConditionID,
			YesTokenID:  yes,
			NoTokenID:   no,
			EndDate:     endDate,
		}, nil
	}
	return nil, nil
}
