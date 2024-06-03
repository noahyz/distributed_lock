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
-- 解锁的时候，
local UNLOCK_LATCH_NAME = KEYS[5]

-- 协程的名字，会做特殊处理，保证全局唯一
local GOROUTINE_NAME = ARGV[1]
--
local UNLOCK_LATCH_EXPIRE_TIME_MS = ARGV[2]
-- 看门狗的过期时间
local WATCHDOG_EXPIRE_TIME_MS = ARGV[3]
-- 当前时间戳
local CURRENT_TIME_MS = ARGV[4]
-- 频道发布的命令
local CHANNEL_PUBLISH_COMMAND = ARGV[5]
-- 频道中解锁的消息
local CHANNEL_UNLOCK_MESSAGE = ARGV[6]

-- 设置一个解锁栅栏，相同的请求限制频率
local res = redis.call('get', UNLOCK_LATCH_NAME);
if res ~= false then
    return tonumber(res);
end ;

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

-- 如果分布式锁的 key(哈希表名字) 不存在，说明没有协程加锁
-- 我们就可以通知队列中等待的协程，过来加锁了
-- 然后给当前协程设置一个解锁阻塞时间，
if (redis.call('exists', KEY_HASH_NAME) == 0) then
    local next_goroutine = redis.call('lindex', GOROUTINE_QUEUE_NAME, 0);
    if next_goroutine ~= false then
        redis.call(CHANNEL_PUBLISH_COMMAND, CHANNEL_PREFIX_NAME .. '_' .. next_goroutine, CHANNEL_UNLOCK_MESSAGE);
    end;
    redis.call('set', UNLOCK_LATCH_NAME, 1, 'px', UNLOCK_LATCH_EXPIRE_TIME_MS);
    return 1;
end;

-- 走到这里，说明分布式锁 key(哈希表) 存在

-- 分布式锁 key 代表的哈希表中，当前协程字段并不存在
-- 也就是说当前协程未加锁，或者当前协程已经解锁但重复解锁。这两种情况直接返回
-- 这种情况不用通知等待的协程解锁，这种情况本来就是非法行为
if (redis.call('hexists', KEY_HASH_NAME, GOROUTINE_NAME) == 0) then
    return nil;
end;

-- 走到这里，说明是当前协程加锁了，我们需要解锁

-- 给分布式锁 key 中当前协程的字段，对应的值减去一。这里是可重入锁的逻辑
local counter = redis.call('hincrby', KEY_HASH_NAME, GOROUTINE_NAME, -1);
if (counter > 0) then
    -- 这里锁还是继续持有，同时设置
    redis.call('pexpire', KEY_HASH_NAME, WATCHDOG_EXPIRE_TIME_MS);
    redis.call('set', UNLOCK_LATCH_NAME, 0, 'px', UNLOCK_LATCH_EXPIRE_TIME_MS);
    return 0;
end;

-- 如果 counter 等于0，说明可以删除这个分布式锁的 key(哈希表)了
redis.call('del', KEY_HASH_NAME);
redis.call('set', UNLOCK_LATCH_NAME, 1, 'px', UNLOCK_LATCH_EXPIRE_TIME_MS);

-- 走到这里，代表解锁成功了，那么此时通知下队列头部等待的协程，可以让其来加锁
local next_goroutine = redis.call('lindex', GOROUTINE_QUEUE_NAME, 0);
if next_goroutine ~= false then
    redis.call(CHANNEL_PUBLISH_COMMAND, CHANNEL_PREFIX_NAME .. '_' .. next_goroutine, CHANNEL_UNLOCK_MESSAGE);
end;

return 1;


-- 返回值说明
-- 返回 nil，表示当前协程并没有加锁，不用解锁
-- 返回 0，表示当前协程解锁了，但是当前协程因为可重入，还没有删除锁
-- 返回 1，表示当前协程解锁了，并且可重入次数用完，锁释放了
-- 对于 unlock_latch 来说，结果也如上 0/1 的场景一样
