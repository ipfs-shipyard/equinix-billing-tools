package equinix

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Equinix struct {
	Token string
}

type Project struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type Projects struct {
	Projects []Project `json:"projects"`
}

type UsageRecord struct {
	Date     string
	Metro    string  `json:"metro"`
	Plan     string  `json:"plan"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Total    float64 `json:"total"`
}

type UsageRecords struct {
	Usages []UsageRecord `json:"usages"`
}

func (eq Equinix) GetProjects() ([]Project, error) {
	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		"https://api.equinix.com/metal/v1/projects?page=1&per_page=1000&include=id,name",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("error while creating HTTP request: %w", err)
	}

	req.Header.Add("X-Auth-Token", eq.Token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error while making the HTTP request: %w", err)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error while reading the response body: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(
			"HTTP error.\nStatus code: %d\nResponse body: %s",
			resp.StatusCode,
			string(bytes),
		)
	}

	var projects Projects

	err = json.Unmarshal(bytes, &projects)
	if err != nil {
		return nil, fmt.Errorf("error while unmarshaling JSON response: %w", err)
	}

	return projects.Projects, nil
}

func (eq Equinix) GetUsages(startDate time.Time, endDate time.Time, projects []Project) (map[Project][]UsageRecord, error) {
	client := &http.Client{}
	usages := make(map[Project][]UsageRecord)

	for _, project := range projects {
		uri := fmt.Sprintf(
			"https://api.equinix.com/metal/v1/projects/%s/usages?created[after]=%sT00:00:00&created[before]=%sT23:59:59.999",
			project.Id,
			startDate.Format("2006-01-02"),
			endDate.Format("2006-01-02"),
		)
		req, err := http.NewRequest(
			"GET",
			uri,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating HTTP request for project %s: %w", project.Id, err)
		}

		req.Header.Add("X-Auth-Token", eq.Token)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error while making the HTTP request for project %s: %w", project.Id, err)
		}
		defer resp.Body.Close()

		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error while reading the response body for project %s: %w", project.Id, err)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf(
				"HTTP error for project %s.\nStatus code: %d\nResponse body: %s",
				project.Id,
				resp.StatusCode,
				string(bytes),
			)
		}

		var records UsageRecords

		err = json.Unmarshal(bytes, &records)
		if err != nil {
			return nil, fmt.Errorf("error while unmarshaling JSON response for project %s: %w", project.Id, err)
		}

		usages[project] = records.Usages
	}

	return usages, nil
}
