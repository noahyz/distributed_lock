-- 分布式锁的 key
-- 存在这个 key，说明分布式锁已经加锁
-- 不存在这个 key，说明分布式锁未加锁
-- 这个 key 是一个哈希。哈希的字段表示协程，字段对应的 value 表示加锁次数（用于实现可重入锁）
local KEY_HASH_NAME = KEYS[1]
-- 协程等待队列，协程无法加锁时，先尾插加入到等待队列中
local GOROUTINE_QUEUE_NAME = KEYS[2]
-- 协程超时集合(有序集合)，每个等待的协程都有他等待的时间，用 score 来表示
local TIMEOUT_SET_NAME = KEYS[3]
-- redis 的频道，用于发布订阅
local CHANNEL_PREFIX_NAME = KEYS[4]

-- 频道发布的命令
local CHANNEL_PUBLISH_COMMAND = ARGV[1]
-- 频道中解锁的消息
local CHANNEL_UNLOCK_MESSAGE = ARGV[2]
-- 当前时间戳
local CURRENT_TIME_MS = ARGV[3]


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

if (redis.call('del', KEY_HASH_NAME) == 1) then
    local next_goroutine = redis.call('lindex', GOROUTINE_QUEUE_NAME, 0);
    if next_goroutine ~= false then
        redis.call(CHANNEL_PUBLISH_COMMAND, CHANNEL_PREFIX_NAME + "_" + next_goroutine, CHANNEL_UNLOCK_MESSAGE);
    end ;
    return 1;
end ;
return 0;
