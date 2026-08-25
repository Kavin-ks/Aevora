package selection

import (
	"testing"

	"github.com/aevora/control-plane/internal/model"
)

func gw(name string, status model.GatewayStatus, active, cap int) model.Gateway {
	return model.Gateway{Name: name, Status: status, ActivePeers: active, Capacity: cap}
}

func TestBest_PicksLeastLoadedHealthy(t *testing.T) {
	cands := []model.Gateway{
		gw("busy", model.GatewayHealthy, 200, 250), // util 0.80
		gw("idle", model.GatewayHealthy, 20, 250),  // util 0.08  <- winner
		gw("mid", model.GatewayHealthy, 125, 250),  // util 0.50
	}
	best, ok := Best(cands)
	if !ok {
		t.Fatal("expected a selection")
	}
	if best.Name != "idle" {
		t.Fatalf("want idle, got %s", best.Name)
	}
}

func TestBest_SkipsUnhealthyAndFull(t *testing.T) {
	cands := []model.Gateway{
		gw("down", model.GatewayUnhealthy, 0, 250), // excluded: unhealthy
		gw("full", model.GatewayHealthy, 250, 250), // excluded: no spare slots
		gw("ok", model.GatewayHealthy, 249, 250),   // only selectable one
	}
	best, ok := Best(cands)
	if !ok {
		t.Fatal("expected a selection")
	}
	if best.Name != "ok" {
		t.Fatalf("want ok, got %s", best.Name)
	}
}

func TestBest_NoneSelectable(t *testing.T) {
	cands := []model.Gateway{
		gw("down", model.GatewayUnhealthy, 0, 250),
		gw("drain", model.GatewayDraining, 0, 250),
	}
	if _, ok := Best(cands); ok {
		t.Fatal("expected no selection when nothing is healthy")
	}
}

func TestFitness_Bounds(t *testing.T) {
	if f := Fitness(gw("empty", model.GatewayHealthy, 0, 100)); f != 1 {
		t.Fatalf("empty gateway fitness want 1, got %v", f)
	}
	if f := Fitness(gw("full", model.GatewayHealthy, 100, 100)); f != 0 {
		t.Fatalf("full gateway fitness want 0, got %v", f)
	}
	if f := Fitness(gw("bad", model.GatewayHealthy, 10, 0)); f != 0 {
		t.Fatalf("zero-capacity fitness want 0, got %v", f)
	}
}
