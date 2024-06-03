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


-- 分布式锁的 key 未加锁的情况下，如果协程等待队列不存在，或者等待队列中头部协程是当前协程
-- 那么当前协程此时可以加锁了
if (redis.call('exists', KEY_HASH_NAME) == 0) and (
        (redis.call('exists', GOROUTINE_QUEUE_NAME) == 0)
                or (redis.call('lindex', GOROUTINE_QUEUE_NAME, 0) == GOROUTINE_NAME)) then

    -- 如果协程等待队列中头部是当前协程，则从队列和集合中移除当前协程
    redis.call('lpop', GOROUTINE_QUEUE_NAME);
    redis.call('zrem', TIMEOUT_SET_NAME, GOROUTINE_NAME);

    -- 获取到目前所有正在等待的协程，减小他们的过期时间
    local all_goroutine = redis.call('zrange', TIMEOUT_SET_NAME, 0, -1);
    for i = 1, #all_goroutine, 1 do
        redis.call('zincrby', TIMEOUT_SET_NAME, -tonumber(GOROUTINE_WAIT_TIME_MS), all_goroutine[i]);
    end ;

    -- 当前协程可以加锁了，设置分布式锁的 key(KEY_HASH_NAME)，表明此时加锁了
    -- 使用当前协程标识作为哈希的字段，此字段对应的 value 为 1，表示是当前协程加锁了，并且第一次加锁，为可重入锁做准备
    redis.call('hset', KEY_HASH_NAME, GOROUTINE_NAME, 1);
    -- 设置分布式锁 key 的过期时间
    redis.call('pexpire', KEY_HASH_NAME, EXPIRE_TIME_MS);
    return nil;
end ;


-- 走到这里，有两种情况
-- 1. 要么分布式锁 KEY_HASH_NAME 已经存在了，说明已经有协程加锁了。协程等待队列的情况此时未判断
-- 2. 要么分布式锁 KEY_HASH_NAME 不存在， 协程等待队列不为空，并且协程等待队列中头部协程不是当前协程


-- 如果分布式锁 KEY_HASH_NAME 已经存在了，说明已经有协程加锁了，此时要判断是否为当前协程加锁了
-- 如果哈希中存在当前协程的字段，说明是当前协程加锁了，此时当前协程还要加锁，那么这里实现可重入锁
if redis.call('hexists', KEY_HASH_NAME, GOROUTINE_NAME) == 1 then
    -- 哈希表中以当前协程为 key，所对应的 value 增加 1，实现了可重入锁
    redis.call('hincrby', KEY_HASH_NAME, GOROUTINE_NAME, 1);
    -- 重新设置分布式锁 key 的过期时间
    redis.call('pexpire', KEY_HASH_NAME, EXPIRE_TIME_MS);
    return nil;
end ;

-- 走到这里
-- 要么已经有其他协程加锁了，当前协程无法加锁
-- 要么是没有协程加锁，但是协程等待队列中的头部协程不是当前协程，因此当前协程也无法加锁

-- 接下来，对于当前线程来说，既然当前协程不能加锁了
-- 那么如果当前协程不在队列中，给他加进去；如果在队列中，返回过期时间即可

-- 如果当前协程在集合中，属于已经在等待的协程了。那么此时获取当前协程的过期时间，然后返回
local expired_ms = redis.call('zscore', TIMEOUT_SET_NAME, GOROUTINE_NAME);
if expired_ms ~= false then
    -- 返回当前协程剩余的时间
    return expired_ms - tonumber(GOROUTINE_WAIT_TIME_MS) - tonumber(CURRENT_TIME_MS);
end ;

-- 走到这里，当前协程不在集合中，说明当前协程需要加入到等待队列中

-- 获取到协程等待队列中尾部的协程
local last_goroutine_name = redis.call('lindex', GOROUTINE_QUEUE_NAME, -1);
local ttl_ms;
-- 如果协程等待队列尾部的协程存在，并且不是当前协程
if last_goroutine_name ~= false and last_goroutine_name ~= GOROUTINE_NAME then
    -- 队列尾部的协程是最晚插入到队列的，他的分数(过期的时间点)也一定是当前队列中最大的。
    -- 队列尾部协程的分数减去当前时间，表示在当前时间的基础上，队列中最晚协程还剩多长时间过期
    ttl_ms = tonumber(redis.call('zscore', TIMEOUT_SET_NAME, last_goroutine_name)) - tonumber(CURRENT_TIME_MS);
else
    -- 如果协程等待队列为空，或者队列的尾部协程是当前协程
    -- 哈希 key 的剩余时间
    ttl_ms = redis.call('pttl', KEY_HASH_NAME);
end ;

-- 构造出当前协程的超时时间
-- 当前时间，加上协程等待时间，再加上当前队列尾部协程的超时时间（如果队列为空，则为哈希 key 的时间）
local curr_goroutine_timeout_ms = ttl_ms + tonumber(GOROUTINE_WAIT_TIME_MS) + tonumber(CURRENT_TIME_MS);
-- 把当前协程加入到队列末尾和集合中
-- 这里我们可以看到有两种情况。集合中有当前协程时，队列不一定有；集合中没有当前协程时，队列一定没有
if redis.call('zadd', TIMEOUT_SET_NAME, curr_goroutine_timeout_ms, GOROUTINE_NAME) == 1 then
    redis.call('rpush', GOROUTINE_QUEUE_NAME, GOROUTINE_NAME);
end ;

return ttl_ms;


-- 对返回值做说明
-- 返回 nil，表示当前协程可以加锁，并且加锁成功了
-- 返回数字，表示当前协程不能加锁，以及当前协程或者这把锁的过期剩余时间
