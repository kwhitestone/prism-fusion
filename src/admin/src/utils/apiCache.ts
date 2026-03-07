/**
 * API 缓存管理器
 * 避免重复请求相同的数据
 */

interface CacheItem<T> {
  data: T;
  timestamp: number;
  expiry: number; // 缓存过期时间（毫秒）
}

class ApiCache {
  private cache = new Map<string, CacheItem<any>>();
  private readonly DEFAULT_EXPIRY = 5 * 60 * 1000; // 5分钟默认过期时间

  /**
   * 获取缓存数据
   */
  get<T>(key: string): T | null {
    const item = this.cache.get(key);
    if (!item) return null;

    const now = Date.now();
    if (now > item.timestamp + item.expiry) {
      this.cache.delete(key);
      return null;
    }

    return item.data;
  }

  /**
   * 设置缓存数据
   */
  set<T>(key: string, data: T, expiry: number = this.DEFAULT_EXPIRY): void {
    this.cache.set(key, {
      data,
      timestamp: Date.now(),
      expiry
    });
  }

  /**
   * 清除指定缓存
   */
  delete(key: string): void {
    this.cache.delete(key);
  }

  /**
   * 清除所有缓存
   */
  clear(): void {
    this.cache.clear();
  }

  /**
   * 清除过期的缓存
   */
  cleanup(): void {
    const now = Date.now();
    for (const [key, item] of this.cache.entries()) {
      if (now > item.timestamp + item.expiry) {
        this.cache.delete(key);
      }
    }
  }
}

// 创建全局缓存实例
export const apiCache = new ApiCache();

// 定期清理过期缓存
setInterval(
  () => {
    apiCache.cleanup();
  },
  5 * 60 * 1000
); // 每5分钟清理一次

// 缓存键常量
export const CACHE_KEYS = {} as const;
