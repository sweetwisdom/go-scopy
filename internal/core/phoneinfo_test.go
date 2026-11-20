package core

import (
	"strings"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func waitPhoneInfo(t *testing.T, s *Service, serial string, done bool) domain.PhoneInfo {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		i, exists := s.State().PhoneInfos[serial]
		if exists && ((done && !i.Loading && !i.UpdatedAt.IsZero()) || (!done && i.Loading)) {
			return i
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("device info state did not transition")
	return domain.PhoneInfo{}
}

func TestPhoneInfoExplicitLoadingAndCompletion(t *testing.T) {
	s := testService(t)
	t.Setenv("GOSCOPY_PHONEINFO_HELPER", "slow")
	s.FetchPhoneInfo("test-serial")
	i := waitPhoneInfo(t, s, "test-serial", false)
	if i.BatteryLevel != -1 || i.MemAvailableKB != -1 {
		t.Fatal("initial unknowns were zero", i)
	}
	s.FetchPhoneInfo("test-serial") // A duplicate read must coalesce.
	i = waitPhoneInfo(t, s, "test-serial", true)
	if i.Error != "" || i.Model != "demo" || i.Manufacturer != "test" || i.StorageTotalBytes != 1000*1024 {
		t.Fatalf("%+v", i)
	}
	// Wait until the coalescing key has also been released before refreshing.
	s.wg.Wait()
	t.Setenv("GOSCOPY_PHONEINFO_HELPER", "fail")
	s.FetchPhoneInfo("test-serial")
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		i = s.State().PhoneInfos["test-serial"]
		if !i.Loading && i.Error != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if i.Loading || !strings.Contains(i.Error, "保留上次结果") || i.Model != "demo" {
		t.Fatalf("refresh lost previous result: %+v", i)
	}
}

func TestPhoneInfoFailedFirstReadCompletes(t *testing.T) {
	s := testService(t)
	t.Setenv("GOSCOPY_PHONEINFO_HELPER", "fail")
	s.FetchPhoneInfo("test-serial")
	i := waitPhoneInfo(t, s, "test-serial", true)
	if i.Error == "" || i.Loading || i.BatteryLevel != -1 || len(i.Warnings) != 8 {
		t.Fatalf("failure stuck loading: %+v", i)
	}
}
