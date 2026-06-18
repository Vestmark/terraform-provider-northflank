// Copyright (c) Vestmark
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vestmark-infra/tf-provider-northflank/internal/nfapi"
)

// SecretFile represents a single secret file or Docker secret mount entry.
type SecretFile struct {
	Data     string `json:"data"`
	Encoding string `json:"encoding"`
}

// SecretRestrictions controls which resources can access a secret group.
type SecretRestrictions struct {
	Restricted        bool
	Tags              []string
	TagMatchCondition string // "and" | "or"
	Services          []string
	Jobs              []string
}

// SecretGroup is the canonical representation of a Northflank project secret
// group, decoupled from verbose generated nfapi type names.
type SecretGroup struct {
	ID                 string
	ProjectID          string
	Name               string
	Description        string
	SecretType         string // "environment" | "arguments" | "environment-arguments"
	Type               string // "secret" | "config"
	Priority           int
	StageID            string
	Tags               []string
	Restrictions       *SecretRestrictions
	Variables          map[string]string
	Files              map[string]SecretFile
	DockerSecretMounts map[string]SecretFile
}

// CreateSecretInput carries the fields for a create request.
type CreateSecretInput struct {
	ProjectID          string
	Name               string
	Description        string
	SecretType         string
	Type               string
	Priority           int
	StageID            string
	Tags               []string
	Restrictions       *SecretRestrictions
	Variables          map[string]string
	Files              map[string]SecretFile
	DockerSecretMounts map[string]SecretFile
}

// UpdateSecretInput carries the fields that may be patched.  Pointer fields are
// omitted from the PATCH body when nil, enabling partial updates.
type UpdateSecretInput struct {
	ProjectID          string
	SecretID           string
	Description        *string
	SecretType         *string
	Type               *string
	Priority           *int
	StageID            *string
	Tags               *[]string
	Restrictions       *SecretRestrictions
	Variables          *map[string]string
	Files              *map[string]SecretFile
	DockerSecretMounts *map[string]SecretFile
}

// CreateProjectSecret creates a new secret group in the given project.
func (c *Client) CreateProjectSecret(ctx context.Context, in CreateSecretInput) (*SecretGroup, error) {
	body := nfapi.PostProjectsSecretsJSONRequestBody{
		Name:       in.Name,
		SecretType: nfapi.PostProjectsSecretsJSONBodySecretType(in.SecretType),
		Priority:   in.Priority,
	}
	if in.Description != "" {
		body.Description = ptrOf(in.Description)
	}
	if in.Type != "" {
		t := nfapi.PostProjectsSecretsJSONBodyType(in.Type)
		body.Type = &t
	}
	if in.StageID != "" {
		body.StageId = ptrOf(in.StageID)
	}
	if len(in.Tags) > 0 {
		body.Tags = &in.Tags
	}
	if in.Restrictions != nil {
		body.Restrictions = buildPostRestrictions(in.Restrictions)
	}
	if len(in.Variables) > 0 || len(in.Files) > 0 || len(in.DockerSecretMounts) > 0 {
		s := &struct {
			DockerSecretMounts *map[string]interface{} `json:"dockerSecretMounts,omitempty"`
			Files              *map[string]interface{} `json:"files,omitempty"`
			Variables          *map[string]string      `json:"variables,omitempty"`
		}{}
		if len(in.Variables) > 0 {
			s.Variables = &in.Variables
		}
		if len(in.Files) > 0 {
			m := toInterfaceMap(in.Files)
			s.Files = &m
		}
		if len(in.DockerSecretMounts) > 0 {
			m := toInterfaceMap(in.DockerSecretMounts)
			s.DockerSecretMounts = &m
		}
		body.Secrets = s
	}

	resp, err := c.api.PostProjectsSecretsWithResponse(ctx, in.ProjectID, body)
	if err != nil {
		return nil, fmt.Errorf("create project secret: %w", err)
	}
	if err := checkStatus(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("create project secret: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("create project secret: empty response body")
	}
	d := resp.JSON200.Data
	sg := &SecretGroup{
		ID:         d.Id,
		ProjectID:  in.ProjectID,
		Name:       d.Name,
		Priority:   d.Priority,
		SecretType: string(d.SecretType),
	}
	if d.Description != nil {
		sg.Description = *d.Description
	}
	if d.Type != nil {
		sg.Type = string(*d.Type)
	}
	if d.StageId != nil {
		sg.StageID = *d.StageId
	}
	if d.Tags != nil {
		sg.Tags = *d.Tags
	}
	if d.Restrictions != nil {
		sg.Restrictions = extractPostRestrictions(d.Restrictions)
	}
	if d.Secrets != nil {
		s := d.Secrets
		if s.Variables != nil {
			sg.Variables = *s.Variables
		}
		if s.Files != nil {
			sg.Files = unmarshalSecretFileMap(*s.Files)
		}
		if s.DockerSecretMounts != nil {
			sg.DockerSecretMounts = unmarshalSecretFileMap(*s.DockerSecretMounts)
		}
	}
	return sg, nil
}

// GetProjectSecret fetches a single secret group.  Returns ErrNotFound on 404.
func (c *Client) GetProjectSecret(ctx context.Context, projectID, secretID string) (*SecretGroup, error) {
	show := nfapi.GetProjectsSecretsSecretidParamsShow("this")
	params := &nfapi.GetProjectsSecretsSecretidParams{
		// "this" → only this group's own secrets, not inherited from addons
		Show: &show,
	}
	resp, err := c.api.GetProjectsSecretsSecretidWithResponse(ctx, projectID, secretID, params)
	if err != nil {
		return nil, fmt.Errorf("get project secret: %w", err)
	}
	if err := checkStatus(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("get project secret %s/%s: %w", projectID, secretID, err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("get project secret: empty response body")
	}
	d := resp.JSON200.Data
	sg := &SecretGroup{
		ID:                 d.Id,
		ProjectID:          d.ProjectId,
		Name:               d.Name,
		Priority:           d.Priority,
		SecretType:         string(d.SecretType),
		Type:               string(d.Type),
		Tags:               d.Tags,
		Variables:          extractStringMap(d.Secrets, "variables"),
		Files:              extractSecretFiles(d.Secrets, "files"),
		DockerSecretMounts: extractSecretFiles(d.Secrets, "dockerSecretMounts"),
		Restrictions:       extractGetRestrictions(d.Restrictions),
	}
	if d.Description != nil {
		sg.Description = *d.Description
	}
	// stageId is absent from the generated GET response struct; extract it
	// defensively from the raw body in case the API returns it.
	var raw struct {
		Data struct {
			StageId *string `json:"stageId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err == nil && raw.Data.StageId != nil {
		sg.StageID = *raw.Data.StageId
	}
	return sg, nil
}

// UpdateProjectSecret partially updates a secret group via PATCH.
func (c *Client) UpdateProjectSecret(ctx context.Context, in UpdateSecretInput) (*SecretGroup, error) {
	body := nfapi.PatchProjectsSecretsSecretidJSONRequestBody{}

	if in.Description != nil {
		body.Description = in.Description
	}
	if in.SecretType != nil {
		st := nfapi.PatchProjectsSecretsSecretidJSONBodySecretType(*in.SecretType)
		body.SecretType = &st
	}
	if in.Type != nil {
		t := nfapi.PatchProjectsSecretsSecretidJSONBodyType(*in.Type)
		body.Type = &t
	}
	if in.Priority != nil {
		body.Priority = in.Priority
	}
	if in.StageID != nil {
		body.StageId = in.StageID
	}
	if in.Tags != nil {
		body.Tags = in.Tags
	}
	if in.Restrictions != nil {
		body.Restrictions = buildPatchRestrictions(in.Restrictions)
	}
	if in.Variables != nil || in.Files != nil || in.DockerSecretMounts != nil {
		s := &struct {
			DockerSecretMounts *map[string]interface{} `json:"dockerSecretMounts,omitempty"`
			Files              *map[string]interface{} `json:"files,omitempty"`
			Variables          *map[string]string      `json:"variables,omitempty"`
		}{}
		if in.Variables != nil {
			s.Variables = in.Variables
		}
		if in.Files != nil {
			m := toInterfaceMap(*in.Files)
			s.Files = &m
		}
		if in.DockerSecretMounts != nil {
			m := toInterfaceMap(*in.DockerSecretMounts)
			s.DockerSecretMounts = &m
		}
		body.Secrets = s
	}

	resp, err := c.api.PatchProjectsSecretsSecretidWithResponse(ctx, in.ProjectID, in.SecretID, body)
	if err != nil {
		return nil, fmt.Errorf("update project secret: %w", err)
	}
	if err := checkStatus(resp.StatusCode(), resp.Body); err != nil {
		return nil, fmt.Errorf("update project secret %s/%s: %w", in.ProjectID, in.SecretID, err)
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("update project secret: empty response body")
	}
	d := resp.JSON200.Data
	sg := &SecretGroup{
		ID:         d.Id,
		ProjectID:  in.ProjectID,
		Name:       d.Name,
		Priority:   d.Priority,
		SecretType: string(d.SecretType),
	}
	if d.Description != nil {
		sg.Description = *d.Description
	}
	if d.Type != nil {
		sg.Type = string(*d.Type)
	}
	if d.StageId != nil {
		sg.StageID = *d.StageId
	}
	if d.Tags != nil {
		sg.Tags = *d.Tags
	}
	if d.Restrictions != nil {
		sg.Restrictions = extractPostRestrictions(d.Restrictions)
	}
	if d.Secrets != nil {
		s := d.Secrets
		if s.Variables != nil {
			sg.Variables = *s.Variables
		}
		if s.Files != nil {
			sg.Files = unmarshalSecretFileMap(*s.Files)
		}
		if s.DockerSecretMounts != nil {
			sg.DockerSecretMounts = unmarshalSecretFileMap(*s.DockerSecretMounts)
		}
	}
	return sg, nil
}

// DeleteProjectSecret deletes a secret group.  A 404 is treated as success.
func (c *Client) DeleteProjectSecret(ctx context.Context, projectID, secretID string) error {
	resp, err := c.api.DeleteProjectsSecretsSecretidWithResponse(ctx, projectID, secretID)
	if err != nil {
		return fmt.Errorf("delete project secret: %w", err)
	}
	if resp.StatusCode() == 404 {
		return nil // already gone — idempotent
	}
	if err := checkStatus(resp.StatusCode(), resp.Body); err != nil {
		return fmt.Errorf("delete project secret %s/%s: %w", projectID, secretID, err)
	}
	return nil
}

// ─── restrictions helpers ─────────────────────────────────────────────────────

func buildPostRestrictions(r *SecretRestrictions) *struct {
	NfObjects *[]struct {
		Id   string                                                      `json:"id"`
		Type nfapi.PostProjectsSecretsJSONBodyRestrictionsNfObjectsType `json:"type"`
	} `json:"nfObjects,omitempty"`
	Restricted        *bool                                                         `json:"restricted,omitempty"`
	TagMatchCondition *nfapi.PostProjectsSecretsJSONBodyRestrictionsTagMatchCondition `json:"tagMatchCondition,omitempty"`
	Tags              *[]string                                                     `json:"tags,omitempty"`
} {
	out := &struct {
		NfObjects *[]struct {
			Id   string                                                      `json:"id"`
			Type nfapi.PostProjectsSecretsJSONBodyRestrictionsNfObjectsType `json:"type"`
		} `json:"nfObjects,omitempty"`
		Restricted        *bool                                                         `json:"restricted,omitempty"`
		TagMatchCondition *nfapi.PostProjectsSecretsJSONBodyRestrictionsTagMatchCondition `json:"tagMatchCondition,omitempty"`
		Tags              *[]string                                                     `json:"tags,omitempty"`
	}{
		Restricted: ptrOf(r.Restricted),
	}
	if len(r.Tags) > 0 {
		out.Tags = &r.Tags
	}
	if r.TagMatchCondition != "" {
		tmc := nfapi.PostProjectsSecretsJSONBodyRestrictionsTagMatchCondition(r.TagMatchCondition)
		out.TagMatchCondition = &tmc
	}
	objs := nfObjectsFromRestrictions[nfapi.PostProjectsSecretsJSONBodyRestrictionsNfObjectsType](r)
	if len(objs) > 0 {
		out.NfObjects = &objs
	}
	return out
}

func buildPatchRestrictions(r *SecretRestrictions) *struct {
	NfObjects *[]struct {
		Id   string                                                                `json:"id"`
		Type nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsNfObjectsType `json:"type"`
	} `json:"nfObjects,omitempty"`
	Restricted        *bool                                                                    `json:"restricted,omitempty"`
	TagMatchCondition *nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsTagMatchCondition `json:"tagMatchCondition,omitempty"`
	Tags              *[]string                                                                `json:"tags,omitempty"`
} {
	out := &struct {
		NfObjects *[]struct {
			Id   string                                                                `json:"id"`
			Type nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsNfObjectsType `json:"type"`
		} `json:"nfObjects,omitempty"`
		Restricted        *bool                                                                    `json:"restricted,omitempty"`
		TagMatchCondition *nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsTagMatchCondition `json:"tagMatchCondition,omitempty"`
		Tags              *[]string                                                                `json:"tags,omitempty"`
	}{
		Restricted: ptrOf(r.Restricted),
	}
	if len(r.Tags) > 0 {
		out.Tags = &r.Tags
	}
	if r.TagMatchCondition != "" {
		tmc := nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsTagMatchCondition(r.TagMatchCondition)
		out.TagMatchCondition = &tmc
	}
	objs := nfObjectsFromRestrictions[nfapi.PatchProjectsSecretsSecretidJSONBodyRestrictionsNfObjectsType](r)
	if len(objs) > 0 {
		out.NfObjects = &objs
	}
	return out
}

// nfObjectsFromRestrictions builds the nfObjects slice from Services and Jobs.
func nfObjectsFromRestrictions[T ~string](r *SecretRestrictions) []struct {
	Id   string `json:"id"`
	Type T      `json:"type"`
} {
	var objs []struct {
		Id   string `json:"id"`
		Type T      `json:"type"`
	}
	for _, id := range r.Services {
		objs = append(objs, struct {
			Id   string `json:"id"`
			Type T      `json:"type"`
		}{Id: id, Type: T("service")})
	}
	for _, id := range r.Jobs {
		objs = append(objs, struct {
			Id   string `json:"id"`
			Type T      `json:"type"`
		}{Id: id, Type: T("job")})
	}
	return objs
}

// extractGetRestrictions maps the GET response restrictions struct to SecretRestrictions.
func extractGetRestrictions(r struct {
	NfObjects *[]struct {
		Id   string                                                                                    `json:"id"`
		Type nfapi.GetProjectsSecretsSecretid200JSONResponseBodyDataRestrictionsNfObjectsType `json:"type"`
	} `json:"nfObjects,omitempty"`
	Restricted        *bool                                                                                          `json:"restricted,omitempty"`
	TagMatchCondition *nfapi.GetProjectsSecretsSecretid200JSONResponseBodyDataRestrictionsTagMatchCondition `json:"tagMatchCondition,omitempty"`
	Tags              *[]string                                                                              `json:"tags,omitempty"`
}) *SecretRestrictions {
	out := &SecretRestrictions{}
	if r.Restricted != nil {
		out.Restricted = *r.Restricted
	}
	if r.Tags != nil {
		out.Tags = *r.Tags
	}
	if r.TagMatchCondition != nil {
		out.TagMatchCondition = string(*r.TagMatchCondition)
	}
	for _, o := range derefSlice(r.NfObjects) {
		switch o.Type {
		case nfapi.GetProjectsSecretsSecretid200JSONResponseBodyDataRestrictionsNfObjectsTypeService:
			out.Services = append(out.Services, o.Id)
		case nfapi.GetProjectsSecretsSecretid200JSONResponseBodyDataRestrictionsNfObjectsTypeJob:
			out.Jobs = append(out.Jobs, o.Id)
		}
	}
	return out
}

// extractPostRestrictions maps a POST/PATCH response restrictions pointer struct to SecretRestrictions.
func extractPostRestrictions[NfObjType ~string, TMCType ~string](r *struct {
	NfObjects *[]struct {
		Id   string    `json:"id"`
		Type NfObjType `json:"type"`
	} `json:"nfObjects,omitempty"`
	Restricted        *bool     `json:"restricted,omitempty"`
	TagMatchCondition *TMCType  `json:"tagMatchCondition,omitempty"`
	Tags              *[]string `json:"tags,omitempty"`
}) *SecretRestrictions {
	if r == nil {
		return nil
	}
	out := &SecretRestrictions{}
	if r.Restricted != nil {
		out.Restricted = *r.Restricted
	}
	if r.Tags != nil {
		out.Tags = *r.Tags
	}
	if r.TagMatchCondition != nil {
		out.TagMatchCondition = string(*r.TagMatchCondition)
	}
	for _, o := range derefSlice(r.NfObjects) {
		switch string(o.Type) {
		case "service":
			out.Services = append(out.Services, o.Id)
		case "job":
			out.Jobs = append(out.Jobs, o.Id)
		}
	}
	return out
}

func derefSlice[T any](s *[]T) []T {
	if s == nil {
		return nil
	}
	return *s
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// toInterfaceMap converts SecretFile values to map[string]interface{} for the
// nfapi request body, which uses interface{} to accept {"data":..,"encoding":..}.
func toInterfaceMap(files map[string]SecretFile) map[string]interface{} {
	result := make(map[string]interface{}, len(files))
	for k, v := range files {
		result[k] = map[string]string{"data": v.Data, "encoding": v.Encoding}
	}
	return result
}

// unmarshalSecretFileMap converts a map[string]interface{} (from nfapi response
// bodies) to map[string]SecretFile via JSON round-trip.
func unmarshalSecretFileMap(raw map[string]interface{}) map[string]SecretFile {
	if len(raw) == 0 {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result map[string]SecretFile
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return result
}

// extractStringMap reads a string-map from a GET response secrets payload
// (map[string]interface{}) by key.
func extractStringMap(secrets map[string]interface{}, key string) map[string]string {
	raw, ok := secrets[key]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return result
}

// extractSecretFiles reads a SecretFile map from a GET response secrets payload
// (map[string]interface{}) by key.
func extractSecretFiles(secrets map[string]interface{}, key string) map[string]SecretFile {
	raw, ok := secrets[key]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result map[string]SecretFile
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	return result
}
