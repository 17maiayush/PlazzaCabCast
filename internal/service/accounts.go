package service

import "ridehailing/internal/domain"

type Users struct {
	repo UserRepo
	now  Clock
}

func NewUsers(repo UserRepo, now Clock) *Users { return &Users{repo: repo, now: now} }

func (s *Users) Register(name string) (domain.User, error) {
	u, err := domain.NewUser(name, s.now())
	if err != nil {
		return domain.User{}, err
	}
	return u, s.repo.Save(u)
}

type Drivers struct {
	repo DriverRepo
}

func NewDrivers(repo DriverRepo) *Drivers { return &Drivers{repo: repo} }

func (s *Drivers) Register(name string, carType domain.CarType, loc domain.Location, rating float64) (domain.Driver, error) {
	d, err := domain.NewDriver(name, carType, loc, rating)
	if err != nil {
		return domain.Driver{}, err
	}
	return d, s.repo.Save(d)
}

func (s *Drivers) UpdateLocation(id string, loc domain.Location) (domain.Driver, error) {
	if err := loc.Validate(); err != nil {
		return domain.Driver{}, err
	}
	if err := s.repo.SetLocation(id, loc); err != nil {
		return domain.Driver{}, err
	}
	return s.repo.Get(id)
}

type Coupons struct {
	repo CouponRepo
}

func NewCoupons(repo CouponRepo) *Coupons { return &Coupons{repo: repo} }

func (s *Coupons) Add(c domain.Coupon) error { return s.repo.Save(c) }

func (s *Coupons) Delete(code string) error { return s.repo.Delete(domain.NormalizeCouponCode(code)) }

func (s *Users) List() []domain.User { return s.repo.List() }

func (s *Drivers) List() []domain.Driver { return s.repo.List() }

func (s *Coupons) List() []domain.Coupon { return s.repo.List() }
