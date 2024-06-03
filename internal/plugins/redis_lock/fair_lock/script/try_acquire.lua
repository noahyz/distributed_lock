-- 分布式锁的 key
-- 存在这个 key，说明分布式锁已经加锁
-- 不存在这个 key，说明分布式锁未加锁
-- 这个 key 是一个哈希。哈希的字段表示协程，字段对应的 value 表示加锁次数（用于实现可重入锁）
local KEY_HASH_NAME = KEYS[1]
-- 协程等待队列，协程无法加锁时，先尾插加入到等待队列中
local GOROUTINE_QUEUE_NAME = KEYS[2]
-- 协程超时集合(有序集合)，每个等待的协程都有他等待的时间，用 score 来表示
local TIMEOUT_SET_NAME = KEYS[3]

-- 协程的名字，会做特殊处理，保证全局唯一
local GOROUTINE_NAME = ARGV[1]
-- 分布式锁 key:KEY_HASH_NAME 的过期时间
local EXPIRE_TIME_MS = ARGV[2]
-- 协程等待的时间
local GOROUTINE_WAIT_TIME_MS = ARGV[3]
-- 当前时间戳
local CURRENT_TIME_MS = ARGV[4]

-- 从队列和集合中移除所有过期的协程
while true do
    -- 从队列中获取到最头部的协程
    local head_goroutine_name = redis.call('lindex', GOROUTINE_QUEUE_NAME, 0);
    if head_goroutine_name == false then
        break ;
    end
    -- 在集合中查询这个协程的过期时间
    local expired_ms = tonumber(redis.call('zscore', TIMEOUT_SET_NAME, head_goroutine_name));
    -- 如果这个协程过期，则从队列和集合中移除这个协程
    -- 否则退出循环，因为队列是依次尾插的，此协程没有过期，表明后插入到队列的协程都没有过期
    if expired_ms <= tonumber(CURRENT_TIME_MS) then
        redis.call('zrem', TIMEOUT_SET_NAME, head_goroutine_name);
        redis.call('lpop', GOROUTINE_QUEUE_NAME);
    else
        break ;
    end ;
end ;

if (redis.call('exists', KEY_HASH_NAME) == 0) and (
        (redis.call('exists', GOROUTINE_QUEUE_NAME) == 0)
                or (redis.call('lindex', GOROUTINE_QUEUE_NAME, 0) == GOROUTINE_NAME)) then
    redis.call('lpop', GOROUTINE_QUEUE_NAME);
    redis.call('zrem', TIMEOUT_SET_NAME, GOROUTINE_NAME);

    local all_goroutine = redis.call('zrange', TIMEOUT_SET_NAME, 0, -1);
    for i = 1, #all_goroutine, 1 do
        redis.call('zincrby', TIMEOUT_SET_NAME, -tonumber(GOROUTINE_WAIT_TIME_MS), all_goroutine[i]);
    end ;

    redis.call('hset', KEY_HASH_NAME, GOROUTINE_NAME, 1);
    redis.call('pexpire', KEY_HASH_NAME, EXPIRE_TIME_MS);
    return nil;
end ;

if (redis.call('hexists', KEY_HASH_NAME, GOROUTINE_NAME) == 1) then
    redis.call('hincrby', KEY_HASH_NAME, GOROUTINE_NAME, 1);
    redis.call('pexpire', KEY_HASH_NAME, EXPIRE_TIME_MS);
    return nil;
end ;

return 1;