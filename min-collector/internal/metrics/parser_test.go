package metrics

import "testing"

const payload = `# HELP minio_node_drive_total_bytes Total bytes
# TYPE minio_node_drive_total_bytes gauge
minio_node_drive_total_bytes{drive="/data1",server="node1:9000"} 1000
minio_node_drive_total_bytes_extra{drive="/data1"} 999
minio_s3_requests_total{api="putobject,copy",server="node1:9000"} 42
minio_cluster_nodes_online_total 4
`

func TestParseFiltersByExactName(t *testing.T) {
	samples := Parse(payload, []string{"minio_node_drive_total_bytes"})
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].Value != "1000" {
		t.Errorf("expected value 1000, got %s", samples[0].Value)
	}
	if got := samples[0].LabelString(); got != "drive=/data1,server=node1:9000" {
		t.Errorf("unexpected labels %q", got)
	}
}

func TestParseAll(t *testing.T) {
	samples := Parse(payload, nil)
	if len(samples) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(samples))
	}
}

func TestParseKeepsCommasInsideLabelValues(t *testing.T) {
	samples := Parse(payload, []string{"minio_s3_requests_total"})
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if got := samples[0].Labels["api"]; got != "putobject,copy" {
		t.Errorf("expected label value to keep the comma, got %q", got)
	}
}

func TestParseSampleWithoutLabels(t *testing.T) {
	samples := Parse(payload, []string{"minio_cluster_nodes_online_total"})
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if samples[0].Value != "4" || samples[0].LabelString() != "" {
		t.Errorf("unexpected sample %+v", samples[0])
	}
}

func TestParseSkipsCommentsAndGarbage(t *testing.T) {
	if samples := Parse("# only a comment\n\nnot_a_sample\n", nil); len(samples) != 0 {
		t.Errorf("expected no samples, got %+v", samples)
	}
}

func TestParseEscapedLabelValue(t *testing.T) {
	samples := Parse(`m{err="a\"b",path="/x"} 1`, nil)
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(samples))
	}
	if got := samples[0].Labels["err"]; got != `a"b` {
		t.Errorf("expected escaped quote to be kept, got %q", got)
	}
	if got := samples[0].Labels["path"]; got != "/x" {
		t.Errorf("expected second label to be parsed, got %q", got)
	}
}
