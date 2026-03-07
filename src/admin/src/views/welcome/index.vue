<script setup lang="ts">
defineOptions({
  name: "Welcome"
});
</script>

<template>
  <div class="welcome-container">
    <div class="welcome-card">
      <div class="prism-logo">
        <svg
          viewBox="0 0 200 200"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <defs>
            <linearGradient
              id="prismGradWelcome"
              x1="0%"
              y1="0%"
              x2="100%"
              y2="100%"
            >
              <stop offset="0%" style="stop-color: #22c55e" />
              <stop offset="50%" style="stop-color: #3b82f6" />
              <stop offset="100%" style="stop-color: #8b5cf6" />
            </linearGradient>
            <linearGradient
              id="beamInWelcome"
              x1="0%"
              y1="50%"
              x2="100%"
              y2="50%"
            >
              <stop
                offset="0%"
                style="stop-color: currentcolor; stop-opacity: 0"
              />
              <stop
                offset="100%"
                style="stop-color: currentcolor; stop-opacity: 0.5"
              />
            </linearGradient>
            <linearGradient
              id="beamGreenWelcome"
              x1="0%"
              y1="0%"
              x2="100%"
              y2="100%"
            >
              <stop offset="0%" style="stop-color: #22c55e" />
              <stop
                offset="100%"
                style="stop-color: #22c55e; stop-opacity: 0.3"
              />
            </linearGradient>
            <linearGradient
              id="beamBlueWelcome"
              x1="0%"
              y1="0%"
              x2="100%"
              y2="100%"
            >
              <stop offset="0%" style="stop-color: #3b82f6" />
              <stop
                offset="100%"
                style="stop-color: #3b82f6; stop-opacity: 0.3"
              />
            </linearGradient>
            <linearGradient
              id="beamPurpleWelcome"
              x1="0%"
              y1="0%"
              x2="100%"
              y2="100%"
            >
              <stop offset="0%" style="stop-color: #8b5cf6" />
              <stop
                offset="100%"
                style="stop-color: #8b5cf6; stop-opacity: 0.3"
              />
            </linearGradient>
            <filter
              id="glowWelcome"
              x="-50%"
              y="-50%"
              width="200%"
              height="200%"
            >
              <feGaussianBlur stdDeviation="3" result="coloredBlur" />
              <feMerge>
                <feMergeNode in="coloredBlur" />
                <feMergeNode in="SourceGraphic" />
              </feMerge>
            </filter>
          </defs>
          <line
            x1="20"
            y1="100"
            x2="70"
            y2="100"
            stroke="url(#beamInWelcome)"
            stroke-width="4"
            stroke-linecap="round"
          />
          <polygon
            points="100,35 150,145 50,145"
            fill="url(#prismGradWelcome)"
            stroke="currentColor"
            stroke-width="2"
            filter="url(#glowWelcome)"
            opacity="0.95"
          />
          <polygon
            points="100,50 135,130 65,130"
            fill="none"
            stroke="currentColor"
            stroke-width="1"
            opacity="0.2"
          />
          <line
            x1="140"
            y1="90"
            x2="180"
            y2="60"
            stroke="url(#beamGreenWelcome)"
            stroke-width="3"
            stroke-linecap="round"
            filter="url(#glowWelcome)"
          />
          <line
            x1="145"
            y1="105"
            x2="185"
            y2="105"
            stroke="url(#beamBlueWelcome)"
            stroke-width="3"
            stroke-linecap="round"
            filter="url(#glowWelcome)"
          />
          <line
            x1="140"
            y1="120"
            x2="180"
            y2="150"
            stroke="url(#beamPurpleWelcome)"
            stroke-width="3"
            stroke-linecap="round"
            filter="url(#glowWelcome)"
          />
          <circle cx="185" cy="55" r="4" fill="#22C55E" opacity="0.8" />
          <circle cx="190" cy="105" r="4" fill="#3B82F6" opacity="0.8" />
          <circle cx="185" cy="155" r="4" fill="#8B5CF6" opacity="0.8" />
        </svg>
      </div>
      <h1 class="title">欢迎使用 Prism Fusion</h1>
      <p class="subtitle">现代化的内容分发管理平台</p>
      <div class="features">
        <div class="feature-item">
          <div class="feature-icon green">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
          </div>
          <span>安全可靠</span>
        </div>
        <div class="feature-item">
          <div class="feature-icon blue">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </div>
          <span>高效分发</span>
        </div>
        <div class="feature-item">
          <div class="feature-icon purple">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
              <line x1="3" y1="9" x2="21" y2="9" />
              <line x1="9" y1="21" x2="9" y2="9" />
            </svg>
          </div>
          <span>简洁易用</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.welcome-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 120px);
  padding: 2rem;
}

.welcome-card {
  max-width: 600px;
  padding: 3rem;
  text-align: center;
  background: var(--el-bg-color);
  border-radius: 24px;
  box-shadow: 0 4px 24px rgb(0 0 0 / 6%);
}

.prism-logo {
  width: 160px;
  height: 160px;
  margin: 0 auto 2rem;
  color: var(--el-text-color-secondary);
}

.prism-logo svg {
  width: 100%;
  height: 100%;
}

.title {
  margin-bottom: 0.75rem;
  font-family:
    "Fira Sans",
    -apple-system,
    BlinkMacSystemFont,
    sans-serif;
  font-size: 2rem;
  font-weight: 700;
  background: linear-gradient(135deg, #22c55e 0%, #3b82f6 50%, #8b5cf6 100%);
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.subtitle {
  margin-bottom: 2.5rem;
  font-size: 1.125rem;
  color: var(--el-text-color-secondary);
}

.features {
  display: flex;
  flex-wrap: wrap;
  gap: 2rem;
  justify-content: center;
}

.feature-item {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  padding: 0.75rem 1.25rem;
  font-size: 0.9375rem;
  color: var(--el-text-color-primary);
  background: var(--el-fill-color-light);
  border-radius: 12px;
}

.feature-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
}

.feature-icon.green {
  color: #22c55e;
  background: rgb(34 197 94 / 10%);
}

.feature-icon.blue {
  color: #3b82f6;
  background: rgb(59 130 246 / 10%);
}

.feature-icon.purple {
  color: #8b5cf6;
  background: rgb(139 92 246 / 10%);
}

@media (width <= 640px) {
  .welcome-card {
    padding: 2rem 1.5rem;
  }

  .title {
    font-size: 1.5rem;
  }

  .features {
    gap: 1rem;
  }

  .feature-item {
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
  }
}
</style>
