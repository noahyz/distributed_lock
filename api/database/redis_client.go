package database

type RedisClient struct {
	Addr     string
	Password string
	DB       string
}

func (r *RedisClient) Ping() error {
	return nil
}

func (r *RedisClient) Subscribe() {

}
