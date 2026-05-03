package sabnzbd

import "context"

// FreeSpace returns the bytes available on SABnzbd's incomplete-job
// directory. We pick diskspace1 (the incomplete dir) over
// diskspace2 (the complete dir) because that is where SAB writes
// during a download — it is the value that determines whether a new
// job will fit. SAB reports both as decimal-gigabyte strings; -1
// is returned when SAB does not surface the field.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	var resp struct {
		FullStatus struct {
			Diskspace1 string `json:"diskspace1"`
		} `json:"fullstatus"`
		// Older SAB builds returned the field at the top level.
		Diskspace1 string `json:"diskspace1"`
	}
	if err := c.getJSON(ctx, c.endpoint("fullstatus", nil), &resp); err != nil {
		return -1, err
	}
	gb := resp.FullStatus.Diskspace1
	if gb == "" {
		gb = resp.Diskspace1
	}
	if gb == "" {
		return -1, nil
	}
	return gigabyteStringToBytes(gb), nil
}

// gigabyteStringToBytes parses SAB's GB-as-string format. Returns
// -1 if the string is unparseable so the caller surfaces "unknown"
// rather than zero (which would falsely advertise a full disk).
func gigabyteStringToBytes(s string) int64 {
	v := mbToBytes(s) // mbToBytes treats input as MB; we want GB → MB → bytes.
	if v == 0 {
		return -1
	}
	return v * 1024
}
