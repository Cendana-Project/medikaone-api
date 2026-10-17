package appointment

import (
	"context"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	doctorrepo "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
)

func (r *Repository) GetAvailabilityDoctor(ctx context.Context, doctorID string) (*response.AvailabilityDoctor, error) {
	return doctorrepo.NewRepository(r.db).GetAvailabilityDoctor(ctx, doctorID)
}
