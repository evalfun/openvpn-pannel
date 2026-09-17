import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    {
      name: 'inject-build-date',
      transformIndexHtml(html) {
        const buildDate = new Date().toLocaleString('zh-CN');
        return html.replace(
          '</head>',
          `<script>
              window.__BUILD_DATE__ = '${buildDate}';
            </script>
            </head>`
        );
      },
    },
  ],
  server: {
    host: "0.0.0.0",
    proxy: {
      '/api': {
        target: 'http://10.0.96.1:8700',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, '/api'),
        cookieDomainRewrite: {
          '*': 'localhost'
        }
      },
    },
  },
})
