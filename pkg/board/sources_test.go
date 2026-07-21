package board

import "testing"

func TestDecodeWeaveDoctorFindings(t *testing.T) {
	raw := []byte(`{"status":"ok","result":{"open":[{"issue":7,"age_seconds":14401,"flags":["needs-steward","stale"]},{"issue":8,"age_seconds":60}]}}`)
	got, err := decodeWeaveDoctorFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got[7].AgeSeconds != 14401 || !got[7].Stale {
		t.Fatalf("stale finding = %+v", got[7])
	}
	if got[8].AgeSeconds != 60 || got[8].Stale {
		t.Fatalf("fresh finding = %+v", got[8])
	}
}
