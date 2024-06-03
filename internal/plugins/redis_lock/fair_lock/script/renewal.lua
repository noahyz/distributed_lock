local KEY_HASH_NAME = KEYS[1]
local GOROUTINE_ID = ARGV[1]
local RENEWAL_TIME_MS = ARGV[2]

if (redis.call('hexists', KEY_HASH_NAME, GOROUTINE_ID) == 1) then
    redis.call('pexpire', KEY_HASH_NAME, RENEWAL_TIME_MS);
    return 1;
end ;
return 0;