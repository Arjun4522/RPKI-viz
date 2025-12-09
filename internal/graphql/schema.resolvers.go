package graphql

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/rpki-viz/backend/internal/model"
)

// ==================== ASN Resolvers ====================

func (r *aSNResolver) Prefixes(ctx context.Context, obj *model.ASN) ([]*model.Prefix, error) {
	if obj == nil {
		return nil, fmt.Errorf("ASN object is nil")
	}

	prefixes, err := r.postgresClient.GetPrefixes(ctx, 0, 0, nil, map[string]interface{}{
		"asnId": obj.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get prefixes for ASN %s: %w", obj.ID, err)
	}

	return prefixes, nil
}

func (r *aSNResolver) Roas(ctx context.Context, obj *model.ASN) ([]*model.ROA, error) {
	if obj == nil {
		return nil, fmt.Errorf("ASN object is nil")
	}

	roas, err := r.postgresClient.GetROAsByASN(ctx, obj.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ROAs for ASN %s: %w", obj.ID, err)
	}

	return roas, nil
}

func (r *aSNResolver) Vrps(ctx context.Context, obj *model.ASN) ([]*model.VRP, error) {
	if obj == nil {
		return nil, fmt.Errorf("ASN object is nil")
	}

	vrps, err := r.postgresClient.GetVRPsByASN(ctx, obj.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VRPs for ASN %s: %w", obj.ID, err)
	}

	return vrps, nil
}

func (r *aSNResolver) ValidationState(ctx context.Context, obj *model.ASN) (model.ValidationState, error) {
	if obj == nil {
		return model.Unknown, fmt.Errorf("ASN object is nil")
	}

	vrps, err := r.postgresClient.GetVRPsByASN(ctx, obj.ID)
	if err != nil {
		return model.Unknown, fmt.Errorf("failed to get VRPs for ASN %s: %w", obj.ID, err)
	}

	if len(vrps) == 0 {
		return model.NotFound, nil
	}

	now := time.Now()
	for _, vrp := range vrps {
		if vrp.NotAfter.After(now) {
			return model.Valid, nil
		}
	}

	return model.Invalid, nil
}

// ==================== GlobalSummary Resolvers ====================

func (r *globalSummaryResolver) ValidationStats(ctx context.Context, obj *model.GlobalSummary) (*ValidationStats, error) {
	if obj == nil {
		return nil, fmt.Errorf("GlobalSummary object is nil")
	}

	return &ValidationStats{
		Valid:    obj.ValidPrefixes,
		Invalid:  obj.InvalidPrefixes,
		NotFound: obj.NotFoundPrefixes,
		Unknown:  obj.TotalPrefixes - obj.ValidPrefixes - obj.InvalidPrefixes - obj.NotFoundPrefixes,
	}, nil
}

// ==================== Prefix Resolvers ====================

func (r *prefixResolver) Asn(ctx context.Context, obj *model.Prefix) (*model.ASN, error) {
	if obj == nil {
		return nil, fmt.Errorf("Prefix object is nil")
	}

	asn, err := r.postgresClient.GetASNByID(ctx, obj.ASNID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ASN for prefix %s: %w", obj.ID, err)
	}

	return asn, nil
}

func (r *prefixResolver) Roa(ctx context.Context, obj *model.Prefix) (*model.ROA, error) {
	if obj == nil {
		return nil, fmt.Errorf("Prefix object is nil")
	}

	roas, err := r.postgresClient.GetROAsByPrefix(ctx, obj.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ROAs for prefix %s: %w", obj.ID, err)
	}

	if len(roas) == 0 {
		return nil, nil
	}

	return roas[0], nil
}

func (r *prefixResolver) Vrp(ctx context.Context, obj *model.Prefix) (*model.VRP, error) {
	if obj == nil {
		return nil, fmt.Errorf("Prefix object is nil")
	}

	vrps, err := r.postgresClient.GetVRPsByPrefix(ctx, obj.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VRPs for prefix %s: %w", obj.ID, err)
	}

	if len(vrps) == 0 {
		return nil, nil
	}

	return vrps[0], nil
}

func (r *prefixResolver) ValidationState(ctx context.Context, obj *model.Prefix) (model.ValidationState, error) {
	if obj == nil {
		return model.Unknown, fmt.Errorf("Prefix object is nil")
	}

	return model.ValidationState(obj.ValidationState), nil
}

// ==================== Query Resolvers ====================

func (r *queryResolver) Asn(ctx context.Context, number int) (*model.ASN, error) {
	return r.postgresClient.GetASNByNumber(ctx, number)
}

func (r *queryResolver) Asns(ctx context.Context, first *int, offset *int, orderBy *ASNOrder, filter *ASNFilter) (*ASNConnection, error) {
	// Set defaults
	limit := 20
	off := 0
	if first != nil && *first > 0 {
		limit = *first
	}
	if offset != nil && *offset > 0 {
		off = *offset
	}

	// Convert orderBy to interface{}
	var order interface{}
	if orderBy != nil {
		order = map[string]interface{}{
			"field":     string(orderBy.Field),
			"direction": string(orderBy.Direction),
		}
	}

	// Convert filter to interface{}
	var filterMap interface{}
	if filter != nil {
		fm := make(map[string]interface{})
		if filter.Number != nil {
			fm["number"] = *filter.Number
		}
		if filter.Name != nil {
			fm["name"] = *filter.Name
		}
		if filter.Country != nil {
			fm["country"] = *filter.Country
		}
		filterMap = fm
	}

	asns, err := r.postgresClient.GetASNs(ctx, limit, off, order, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get ASNs: %w", err)
	}

	edges := make([]*ASNEdge, len(asns))
	for i, asn := range asns {
		edges[i] = &ASNEdge{
			Node:   asn,
			Cursor: encodeCursor(asn.ID),
		}
	}

	hasNextPage := len(asns) == limit
	hasPreviousPage := off > 0

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &ASNConnection{
		Edges: edges,
		PageInfo: &PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(asns),
	}, nil
}

func (r *queryResolver) Prefix(ctx context.Context, cidr string) (*model.Prefix, error) {
	return r.postgresClient.GetPrefixByCIDR(ctx, cidr)
}

func (r *queryResolver) Prefixes(ctx context.Context, first *int, offset *int, orderBy *PrefixOrder, filter *PrefixFilter) (*PrefixConnection, error) {
	limit := 20
	off := 0
	if first != nil && *first > 0 {
		limit = *first
	}
	if offset != nil && *offset > 0 {
		off = *offset
	}

	var order interface{}
	if orderBy != nil {
		order = map[string]interface{}{
			"field":     string(orderBy.Field),
			"direction": string(orderBy.Direction),
		}
	}

	var filterMap interface{}
	if filter != nil {
		fm := make(map[string]interface{})
		if filter.Cidr != nil {
			fm["cidr"] = *filter.Cidr
		}
		if filter.Asn != nil {
			asn, err := r.postgresClient.GetASNByNumber(ctx, *filter.Asn)
			if err != nil {
				return nil, fmt.Errorf("failed to get ASN: %w", err)
			}
			if asn != nil {
				fm["asnId"] = asn.ID
			}
		}
		if filter.ValidationState != nil {
			fm["validationState"] = string(*filter.ValidationState)
		}
		filterMap = fm
	}

	prefixes, err := r.postgresClient.GetPrefixes(ctx, limit, off, order, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get prefixes: %w", err)
	}

	edges := make([]*PrefixEdge, len(prefixes))
	for i, prefix := range prefixes {
		edges[i] = &PrefixEdge{
			Node:   prefix,
			Cursor: encodeCursor(prefix.ID),
		}
	}

	hasNextPage := len(prefixes) == limit
	hasPreviousPage := off > 0

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &PrefixConnection{
		Edges: edges,
		PageInfo: &PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(prefixes),
	}, nil
}

func (r *queryResolver) Roa(ctx context.Context, id string) (*model.ROA, error) {
	return r.postgresClient.GetROAByID(ctx, id)
}

func (r *queryResolver) Roas(ctx context.Context, first *int, offset *int, orderBy *ROAOrder, filter *ROAFilter) (*ROAConnection, error) {
	limit := 20
	off := 0
	if first != nil && *first > 0 {
		limit = *first
	}
	if offset != nil && *offset > 0 {
		off = *offset
	}

	var order interface{}
	if orderBy != nil {
		order = map[string]interface{}{
			"field":     string(orderBy.Field),
			"direction": string(orderBy.Direction),
		}
	}

	var filterMap interface{}
	if filter != nil {
		fm := make(map[string]interface{})
		if filter.Asn != nil {
			fm["asn"] = *filter.Asn
		}
		if filter.Prefix != nil {
			fm["prefix"] = *filter.Prefix
		}
		if filter.Tal != nil {
			fm["talId"] = *filter.Tal
		}
		filterMap = fm
	}

	roas, err := r.postgresClient.GetROAs(ctx, limit, off, order, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get ROAs: %w", err)
	}

	edges := make([]*ROAEdge, len(roas))
	for i, roa := range roas {
		edges[i] = &ROAEdge{
			Node:   roa,
			Cursor: encodeCursor(roa.ID),
		}
	}

	hasNextPage := len(roas) == limit
	hasPreviousPage := off > 0

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &ROAConnection{
		Edges: edges,
		PageInfo: &PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(roas),
	}, nil
}

func (r *queryResolver) Vrp(ctx context.Context, id string) (*model.VRP, error) {
	return r.postgresClient.GetVRPByID(ctx, id)
}

func (r *queryResolver) Vrps(ctx context.Context, first *int, offset *int, orderBy *VRPOrder, filter *VRPFilter) (*VRPConnection, error) {
	limit := 20
	off := 0
	if first != nil && *first > 0 {
		limit = *first
	}
	if offset != nil && *offset > 0 {
		off = *offset
	}

	var order interface{}
	if orderBy != nil {
		order = map[string]interface{}{
			"field":     string(orderBy.Field),
			"direction": string(orderBy.Direction),
		}
	}

	var filterMap interface{}
	if filter != nil {
		fm := make(map[string]interface{})
		if filter.Asn != nil {
			asn, err := r.postgresClient.GetASNByNumber(ctx, *filter.Asn)
			if err != nil {
				return nil, fmt.Errorf("failed to get ASN: %w", err)
			}
			if asn != nil {
				fm["asnId"] = asn.ID
			}
		}
		if filter.Prefix != nil {
			prefix, err := r.postgresClient.GetPrefixByCIDR(ctx, *filter.Prefix)
			if err != nil {
				return nil, fmt.Errorf("failed to get prefix: %w", err)
			}
			if prefix != nil {
				fm["prefixId"] = prefix.ID
			}
		}
		filterMap = fm
	}

	vrps, err := r.postgresClient.GetVRPs(ctx, limit, off, order, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get VRPs: %w", err)
	}

	edges := make([]*VRPEdge, len(vrps))
	for i, vrp := range vrps {
		edges[i] = &VRPEdge{
			Node:   vrp,
			Cursor: encodeCursor(vrp.ID),
		}
	}

	hasNextPage := len(vrps) == limit
	hasPreviousPage := off > 0

	var startCursor, endCursor *string
	if len(edges) > 0 {
		start := edges[0].Cursor
		end := edges[len(edges)-1].Cursor
		startCursor = &start
		endCursor = &end
	}

	return &VRPConnection{
		Edges: edges,
		PageInfo: &PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: hasPreviousPage,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(vrps),
	}, nil
}

func (r *queryResolver) TrustAnchor(ctx context.Context, id string) (*model.TrustAnchor, error) {
	return r.postgresClient.GetTrustAnchorByID(ctx, id)
}

func (r *queryResolver) TrustAnchors(ctx context.Context) ([]*model.TrustAnchor, error) {
	return r.postgresClient.GetTrustAnchors(ctx)
}

func (r *queryResolver) GlobalSummary(ctx context.Context) (*model.GlobalSummary, error) {
	// Check cache first
	if r.redisClient != nil {
		if cached, err := r.redisClient.GetCachedGlobalSummary(); err == nil && cached != nil {
			if summary, ok := cached.(*model.GlobalSummary); ok {
				return summary, nil
			}
		}
	}

	summary, err := r.postgresClient.GetGlobalSummary(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get global summary: %w", err)
	}

	// Cache the result
	if r.redisClient != nil {
		r.redisClient.CacheGlobalSummary(summary, 5*time.Minute)
	}

	return summary, nil
}

func (r *queryResolver) ValidatePrefix(ctx context.Context, asn int, prefix string) (*model.ValidationResponse, error) {
	// Check cache first
	if r.redisClient != nil {
		if cached, err := r.redisClient.GetCachedValidationResult(asn, prefix); err == nil && cached != nil {
			if response, ok := cached.(*model.ValidationResponse); ok {
				return response, nil
			}
		}
	}

	// Get ASN to get its ID
	asnModel, err := r.postgresClient.GetASNByNumber(ctx, asn)
	if err != nil {
		return nil, fmt.Errorf("failed to get ASN: %w", err)
	}
	if asnModel == nil {
		return &model.ValidationResponse{
			ASN:    asn,
			Prefix: prefix,
			State:  model.NotFound,
			Reason: "ASN not found",
		}, nil
	}

	// Get prefix to get its ID
	prefixModel, err := r.postgresClient.GetPrefixByCIDR(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get prefix: %w", err)
	}
	if prefixModel == nil {
		return &model.ValidationResponse{
			ASN:    asn,
			Prefix: prefix,
			State:  model.NotFound,
			Reason: "Prefix not found in database",
		}, nil
	}

	// Get VRPs for this ASN and prefix
	vrps, err := r.postgresClient.GetVRPsByPrefix(ctx, prefixModel.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VRPs for prefix: %w", err)
	}

	// Filter VRPs to only those for the ASN
	var asnVrps []*model.VRP
	for _, vrp := range vrps {
		if vrp.ASNID == asnModel.ID {
			asnVrps = append(asnVrps, vrp)
		}
	}

	result := r.prefixValidator.ValidatePrefix(asn, prefix, asnVrps)

	response := &model.ValidationResponse{
		ASN:    asn,
		Prefix: prefix,
		State:  result.State,
		Reason: result.Reason,
	}

	// Cache the result
	if r.redisClient != nil {
		r.redisClient.CacheValidationResult(asn, prefix, response, 15*time.Minute)
	}

	return response, nil
}

// ==================== ROA Resolvers ====================

func (r *rOAResolver) Asn(ctx context.Context, obj *model.ROA) (*model.ASN, error) {
	if obj == nil {
		return nil, fmt.Errorf("ROA object is nil")
	}

	return r.postgresClient.GetASNByID(ctx, obj.ASNID)
}

func (r *rOAResolver) Prefix(ctx context.Context, obj *model.ROA) (*model.Prefix, error) {
	if obj == nil {
		return nil, fmt.Errorf("ROA object is nil")
	}

	return r.postgresClient.GetPrefixByID(ctx, obj.PrefixID)
}

func (r *rOAResolver) Validity(ctx context.Context, obj *model.ROA) (*Validity, error) {
	if obj == nil {
		return nil, fmt.Errorf("ROA object is nil")
	}

	return &Validity{
		NotBefore: obj.NotBefore,
		NotAfter:  obj.NotAfter,
	}, nil
}

func (r *rOAResolver) Certificates(ctx context.Context, obj *model.ROA) ([]*model.Certificate, error) {
	if obj == nil {
		return nil, fmt.Errorf("ROA object is nil")
	}

	return r.postgresClient.GetCertificatesByROA(ctx, obj.ID)
}

func (r *rOAResolver) Tal(ctx context.Context, obj *model.ROA) (*model.TrustAnchor, error) {
	if obj == nil {
		return nil, fmt.Errorf("ROA object is nil")
	}

	return r.postgresClient.GetTrustAnchorByID(ctx, obj.TALID)
}

// ==================== TrustAnchor Resolvers ====================

func (r *trustAnchorResolver) Certificates(ctx context.Context, obj *model.TrustAnchor) ([]*model.Certificate, error) {
	if obj == nil {
		return nil, fmt.Errorf("TrustAnchor object is nil")
	}

	return r.postgresClient.GetCertificatesByTrustAnchor(ctx, obj.ID)
}

// ==================== VRP Resolvers ====================

func (r *vRPResolver) Asn(ctx context.Context, obj *model.VRP) (*model.ASN, error) {
	if obj == nil {
		return nil, fmt.Errorf("VRP object is nil")
	}

	return r.postgresClient.GetASNByID(ctx, obj.ASNID)
}

func (r *vRPResolver) Prefix(ctx context.Context, obj *model.VRP) (*model.Prefix, error) {
	if obj == nil {
		return nil, fmt.Errorf("VRP object is nil")
	}

	return r.postgresClient.GetPrefixByID(ctx, obj.PrefixID)
}

func (r *vRPResolver) Validity(ctx context.Context, obj *model.VRP) (*Validity, error) {
	if obj == nil {
		return nil, fmt.Errorf("VRP object is nil")
	}

	return &Validity{
		NotBefore: obj.NotBefore,
		NotAfter:  obj.NotAfter,
	}, nil
}

func (r *vRPResolver) Roa(ctx context.Context, obj *model.VRP) (*model.ROA, error) {
	if obj == nil {
		return nil, fmt.Errorf("VRP object is nil")
	}

	return r.postgresClient.GetROAByID(ctx, obj.ROAID)
}

// ==================== ValidationResponse Resolvers ====================

func (r *validationResponseResolver) MatchedVRPs(ctx context.Context, obj *model.ValidationResponse) ([]*model.VRP, error) {
	if obj == nil {
		return nil, fmt.Errorf("ValidationResponse object is nil")
	}

	// Get VRPs that match this ASN and prefix
	filterMap := map[string]interface{}{
		"asn":    obj.ASN,
		"prefix": obj.Prefix,
	}

	vrps, err := r.postgresClient.GetVRPs(ctx, 0, 0, nil, filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to get matched VRPs: %w", err)
	}

	return vrps, nil
}

// ==================== Resolver Implementations ====================

func (r *Resolver) ASN() ASNResolver                     { return &aSNResolver{r} }
func (r *Resolver) GlobalSummary() GlobalSummaryResolver { return &globalSummaryResolver{r} }
func (r *Resolver) Prefix() PrefixResolver               { return &prefixResolver{r} }
func (r *Resolver) Query() QueryResolver                 { return &queryResolver{r} }
func (r *Resolver) ROA() ROAResolver                     { return &rOAResolver{r} }
func (r *Resolver) TrustAnchor() TrustAnchorResolver     { return &trustAnchorResolver{r} }
func (r *Resolver) VRP() VRPResolver                     { return &vRPResolver{r} }
func (r *Resolver) ValidationResponse() ValidationResponseResolver {
	return &validationResponseResolver{r}
}

type aSNResolver struct{ *Resolver }
type globalSummaryResolver struct{ *Resolver }
type prefixResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
type rOAResolver struct{ *Resolver }
type trustAnchorResolver struct{ *Resolver }
type vRPResolver struct{ *Resolver }
type validationResponseResolver struct{ *Resolver }

// ==================== Helper Functions ====================

// encodeCursor creates a cursor from an ID
func encodeCursor(id string) string {
	return base64.StdEncoding.EncodeToString([]byte(id))
}

// decodeCursor decodes a cursor to an ID
func decodeCursor(cursor string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// parseCursorToOffset converts a cursor to an offset
func parseCursorToOffset(cursor string) (int, error) {
	id, err := decodeCursor(cursor)
	if err != nil {
		return 0, err
	}
	// This is a simple implementation - you might want to store actual offsets
	offset, err := strconv.Atoi(id)
	if err != nil {
		return 0, nil
	}
	return offset, nil
}
