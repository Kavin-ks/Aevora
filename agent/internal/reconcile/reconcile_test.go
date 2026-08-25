package reconcile

import (
	"reflect"
	"testing"
)

func TestDiff_AddNewAndRemoveStale(t *testing.T) {
	current := []Peer{
		{PublicKey: "keep", AllowedIPs: []string{"10.7.0.2/32"}},
		{PublicKey: "stale", AllowedIPs: []string{"10.7.0.9/32"}},
	}
	desired := []Peer{
		{PublicKey: "keep", AllowedIPs: []string{"10.7.0.2/32"}},
		{PublicKey: "new", AllowedIPs: []string{"10.7.0.3/32"}},
	}
	plan := Diff(current, desired)

	if len(plan.Add) != 1 || plan.Add[0].PublicKey != "new" {
		t.Fatalf("Add = %+v, want [new]", plan.Add)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"stale"}) {
		t.Fatalf("Remove = %v, want [stale]", plan.Remove)
	}
}

func TestDiff_NoChangeIsEmpty(t *testing.T) {
	peers := []Peer{{PublicKey: "a", AllowedIPs: []string{"10.7.0.2/32", "fd07::2/128"}}}
	// Same peer, allowed-ips in a different order — must be treated as equal.
	current := []Peer{{PublicKey: "a", AllowedIPs: []string{"fd07::2/128", "10.7.0.2/32"}}}
	if plan := Diff(current, peers); !plan.Empty() {
		t.Fatalf("expected empty plan, got %+v", plan)
	}
}

func TestDiff_AllowedIPsChangedReAdds(t *testing.T) {
	current := []Peer{{PublicKey: "a", AllowedIPs: []string{"10.7.0.2/32"}}}
	desired := []Peer{{PublicKey: "a", AllowedIPs: []string{"10.7.0.5/32"}}}
	plan := Diff(current, desired)
	if len(plan.Add) != 1 || plan.Add[0].AllowedIPs[0] != "10.7.0.5/32" {
		t.Fatalf("expected re-add with new ips, got %+v", plan.Add)
	}
	if len(plan.Remove) != 0 {
		t.Fatalf("should not remove a peer that is being updated, got %v", plan.Remove)
	}
}

func TestDiff_DeterministicOrder(t *testing.T) {
	desired := []Peer{
		{PublicKey: "c", AllowedIPs: []string{"10.7.0.4/32"}},
		{PublicKey: "a", AllowedIPs: []string{"10.7.0.2/32"}},
		{PublicKey: "b", AllowedIPs: []string{"10.7.0.3/32"}},
	}
	plan := Diff(nil, desired)
	got := []string{plan.Add[0].PublicKey, plan.Add[1].PublicKey, plan.Add[2].PublicKey}
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("Add order = %v, want sorted", got)
	}
}
