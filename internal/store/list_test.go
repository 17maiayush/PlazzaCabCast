package store_test

import (
	"testing"
	"time"

	"ridehailing/internal/domain"
	"ridehailing/internal/store"
)

func TestUsersListOldestFirst(t *testing.T) {
	users := store.NewUsers()
	if got := users.List(); len(got) != 0 {
		t.Fatalf("empty store listed %v", got)
	}
	later, _ := domain.NewUser("Zoya", now.Add(time.Minute))
	earlier, _ := domain.NewUser("Asha", now)
	for _, u := range []domain.User{later, earlier} {
		if err := users.Save(u); err != nil {
			t.Fatal(err)
		}
	}
	got := users.List()
	if len(got) != 2 || got[0].ID != earlier.ID || got[1].ID != later.ID {
		t.Fatalf("List = %+v, want oldest first", got)
	}
}

func TestDriversListByName(t *testing.T) {
	drivers := store.NewDrivers()
	meena, _ := domain.NewDriver("Meena", domain.Sedan, origin, 4)
	arun, _ := domain.NewDriver("Arun", domain.Hatchback, origin, 4)
	for _, d := range []domain.Driver{meena, arun} {
		if err := drivers.Save(d); err != nil {
			t.Fatal(err)
		}
	}
	got := drivers.List()
	if len(got) != 2 || got[0].Name != "Arun" || got[1].Name != "Meena" {
		t.Fatalf("List = %+v, want sorted by name", got)
	}
}

func TestCouponsListByCodeAndCopied(t *testing.T) {
	coupons := store.NewCoupons()
	exp := now
	b, _ := domain.NewPercentCoupon("B", 10, &exp)
	a, _ := domain.NewFlatCoupon("A", 100, nil)
	for _, c := range []domain.Coupon{b, a} {
		if err := coupons.Save(c); err != nil {
			t.Fatal(err)
		}
	}
	got := coupons.List()
	if len(got) != 2 || got[0].Code != "A" || got[1].Code != "B" {
		t.Fatalf("List = %+v, want sorted by code", got)
	}
	*got[1].ExpiresAt = now.Add(time.Hour)
	if again := coupons.List(); !again[1].ExpiresAt.Equal(now) {
		t.Fatal("List returned coupons that alias the stored ones")
	}
}
