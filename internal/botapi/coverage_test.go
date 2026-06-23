package botapi

import "testing"

func TestCoverageReportsEveryRegisteredMethod(t *testing.T) {
	report := Coverage("compat")
	if report.APIVersion != BotAPIVersion {
		t.Fatalf("APIVersion = %q, want %q", report.APIVersion, BotAPIVersion)
	}
	if report.Summary.Total != len(botAPIMethodNames) {
		t.Fatalf("summary total = %d, want %d", report.Summary.Total, len(botAPIMethodNames))
	}
	if len(report.Methods) != len(botAPIMethodNames) {
		t.Fatalf("methods length = %d, want %d", len(report.Methods), len(botAPIMethodNames))
	}

	counted := report.Summary.Stateful + report.Summary.UIRendered + report.Summary.CompatibilityStub + report.Summary.NotYetSemantic
	if counted != report.Summary.Total {
		t.Fatalf("coverage counts add to %d, want %d", counted, report.Summary.Total)
	}
}

func TestMethodCoverageCanonicalizesCase(t *testing.T) {
	coverage, ok := MethodCoverage("SENDMESSAGE")
	if !ok {
		t.Fatal("MethodCoverage returned ok=false for SENDMESSAGE")
	}
	if coverage.Name != "sendMessage" || coverage.Level != CoverageStateful {
		t.Fatalf("coverage = %#v, want stateful sendMessage", coverage)
	}
}

func TestStrictSupportsImplementedOrRenderedMethodsOnly(t *testing.T) {
	if !StrictSupports("sendMessage") {
		t.Fatal("StrictSupports(sendMessage) = false, want true")
	}
	if !StrictSupports("sendVideo") {
		t.Fatal("StrictSupports(sendVideo) = false, want true for UI-rendered generic send")
	}
	if StrictSupports("sendInvoice") {
		t.Fatal("StrictSupports(sendInvoice) = true, want false for not-yet-semantic payment flow")
	}
	if StrictSupports("banChatMember") {
		t.Fatal("StrictSupports(banChatMember) = true, want false for compatibility stub")
	}
}
