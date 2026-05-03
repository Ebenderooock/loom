package sabnzbd

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// Add submits a job to SABnzbd. NZB-by-URL takes the addurl path;
// raw bytes go through addfile (multipart). Returns the assigned
// nzo_id as the per-client item ID.
//
// SABnzbd 3.x always returns the nzo_id list on success; we surface
// the first one. Empty lists are mapped to ErrServer because that
// state means SAB accepted the request but failed to enqueue it.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	switch {
	case len(req.RawBytes) > 0:
		return c.addFile(ctx, req)
	case req.NZBURL != "":
		return c.addURL(ctx, req)
	default:
		return downloads.AddResult{}, fmt.Errorf("%w: AddRequest has neither NZBURL nor RawBytes", ErrMalformedNZB)
	}
}

// addURLResponse is the parsed envelope from the addurl/addfile
// endpoints: SAB returns the assigned nzo_ids alongside status:true.
type addURLResponse struct {
	Status bool     `json:"status"`
	NzoIDs []string `json:"nzo_ids"`
}

// addURL hands SABnzbd a fetchable NZB URL. SAB grabs the file
// itself; we never download it.
func (c *Client) addURL(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	form := commonAddForm(req)
	form.Set("name", req.NZBURL)
	target := c.endpoint("addurl", nil)
	var resp addURLResponse
	if err := c.postForm(ctx, target, form, &resp); err != nil {
		return downloads.AddResult{}, err
	}
	return c.firstNzo(resp)
}

// addFile uploads NZB bytes via multipart/form-data. The field name
// must be "name"; SAB uses the file's filename to derive the job
// title when nzbname is unset.
func (c *Client) addFile(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

	for k, vs := range commonAddForm(req) {
		for _, v := range vs {
			if err := mw.WriteField(k, v); err != nil {
				return downloads.AddResult{}, fmt.Errorf("%w: multipart field %q: %s", ErrUpstream, k, err.Error())
			}
		}
	}

	filename := req.Title
	if filename == "" {
		filename = "upload.nzb"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".nzb") {
		filename += ".nzb"
	}
	fw, err := mw.CreateFormFile("name", filename)
	if err != nil {
		return downloads.AddResult{}, fmt.Errorf("%w: multipart file: %s", ErrUpstream, err.Error())
	}
	if _, err := fw.Write(req.RawBytes); err != nil {
		return downloads.AddResult{}, fmt.Errorf("%w: multipart write: %s", ErrUpstream, err.Error())
	}
	if err := mw.Close(); err != nil {
		return downloads.AddResult{}, fmt.Errorf("%w: multipart close: %s", ErrUpstream, err.Error())
	}

	target := c.endpoint("addfile", nil)
	var resp addURLResponse
	if err := c.postMultipart(ctx, target, body, mw.FormDataContentType(), &resp); err != nil {
		return downloads.AddResult{}, err
	}
	return c.firstNzo(resp)
}

// commonAddForm populates the SAB params shared between addurl and
// addfile: cat, priority, pp, script, nzbname. Empty values are
// omitted so SAB falls back to its category defaults.
func commonAddForm(req downloads.AddRequest) url.Values {
	form := url.Values{}
	if req.Category != "" {
		form.Set("cat", req.Category)
	}
	if req.Title != "" {
		form.Set("nzbname", req.Title)
	}
	// Tags map to SAB's per-job script and priority overrides via a
	// small convention so callers can drive both without growing
	// AddRequest. The two recognised prefixes mirror the SAB API
	// fields verbatim.
	for _, tag := range req.Tags {
		switch {
		case strings.HasPrefix(tag, "priority="):
			form.Set("priority", strings.TrimPrefix(tag, "priority="))
		case strings.HasPrefix(tag, "script="):
			form.Set("script", strings.TrimPrefix(tag, "script="))
		case strings.HasPrefix(tag, "pp="):
			form.Set("pp", strings.TrimPrefix(tag, "pp="))
		}
	}
	return form
}

// firstNzo extracts the first nzo_id, or maps an empty list to
// ErrServer because SAB returns status:true with empty nzo_ids when
// it silently dropped a job.
func (c *Client) firstNzo(resp addURLResponse) (downloads.AddResult, error) {
	if !resp.Status || len(resp.NzoIDs) == 0 {
		return downloads.AddResult{}, fmt.Errorf("%w: SABnzbd returned no nzo_id", ErrServer)
	}
	return downloads.AddResult{ClientID: c.id, ItemID: resp.NzoIDs[0]}, nil
}
