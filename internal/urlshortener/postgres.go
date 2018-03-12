package urlshortener

import (
	"database/sql"
	"fmt"

	// This loads the postgres drivers.
	_ "github.com/lib/pq"
)

// shortURLPostgresRepository is an in-memory user database.
type shortURLPostgresRepository struct {
	db *sql.DB
}

// New returns a postgres backed storage service.
func newPostgresStorage(host, port, user, password, dbName string) (shortURLStorage, error) {
	// Connect postgres
	connect := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbName)

	db, err := sql.Open("postgres", connect)
	if err != nil {
		return nil, err
	}

	// Ping to connection
	err = db.Ping()
	if err != nil {
		return nil, err
	}

	// Create table if not exists
	strQuery := "CREATE TABLE IF NOT EXISTS shortener (uid serial NOT NULL, url VARCHAR not NULL, " +
		"visited boolean DEFAULT FALSE, count INTEGER DEFAULT 0);"

	_, err = db.Exec(strQuery)
	if err != nil {
		return nil, err
	}
	return &shortURLPostgresRepository{db}, nil
}

// ByShortURL finds and URL in our databse.
func (u *shortURLInMemoryRepository) ByURL(URL string) (*shortURL, error) {

	for _, mapping := range u.shortURLRepository {
		if mapping.URL == URL {
			return mapping, nil
		}
	}
	return nil, errURLNotFound
}

func (u *shortURLInMemoryRepository) ByID(id string) (*shortURL, error) {

	key, err := base62.Decode(id)
	if err != nil {
		return nil, errMalformedURL
	}
	for _, mapping := range u.shortURLRepository {
		if mapping.ID == key {
			return mapping, nil
		}
	}
	return nil, errURLNotFound
}

// ByShortURL finds and URL in our databse.
func (u *shortURLInMemoryRepository) Save(item *shortURL) (*shortURL, error) {

	m, err := u.ByURL(item.URL)
	if err != errURLNotFound {
		return m, err
	}
	var mapping shortURL
	var autoInc uint64
	for _, mapping := range u.shortURLRepository {
		if mapping.ID > autoInc {
			autoInc = mapping.ID
		}
	}
	autoInc++
	mapping.URL = item.URL
	mapping.VisitsCounter = 0
	mapping.ID = autoInc
	encodedkey := base62.Encode(mapping.ID)
	u.shortURLRepository[encodedkey] = &mapping
	return &mapping, nil
}

/*

func (p *shortURLPostgresRepository) Save(url string) (string, error) {
	var id int64
	err := p.db.QueryRow("INSERT INTO shortener(url,visited,count) VALUES($1,$2,$3) returning uid;", url, false, 0).Scan(&id)
	if err != nil {
		return "", err
	}
	return base62.Encode(id), nil
}

func (p *shortURLPostgresRepository) Load(code string) (string, error) {
	id, err := base62.Decode(code)
	if err != nil {
		return "", err
	}

	var url string
	err = p.db.QueryRow("update shortener set visited=true, count = count + 1 where uid=$1 RETURNING url", id).Scan(&url)
	if err != nil {
		return "", err
	}
	return url, nil
}

func (p *shortURLPostgresRepository) LoadInfo(code string) (*shortURL, error) {
	id, err := base62.Decode(code)
	if err != nil {
		return nil, err
	}

	var item storage.Item
	err = p.db.QueryRow("SELECT url, visited, count FROM shortener where uid=$1 limit 1", id).
		Scan(&item.URL, &item.Visited, &item.Count)
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (p *shortURLPostgresRepository) Close() error { return p.db.Close() }
*/
