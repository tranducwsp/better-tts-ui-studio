import http from 'http';
import { exec } from 'child_process';
import path from 'path';

const PORT = process.env.PORT || 3001;
let isBuilding = false;

console.log('🚀 Starting FE Builder Service on port', PORT);

// Initial build on startup
triggerBuild();

const server = http.createServer((req, res) => {
  res.setHeader('Content-Type', 'application/json');

  if (req.method === 'POST' && (req.url === '/rebuild' || req.url === '/api/internal/rebuild')) {
    if (isBuilding) {
      res.writeHead(429);
      res.end(JSON.stringify({ status: 'warning', message: 'Build already in progress' }));
      return;
    }

    res.writeHead(202);
    res.end(JSON.stringify({ status: 'ok', message: 'Build process triggered asynchronously' }));
    
    // Trigger build asynchronously
    triggerBuild();
    return;
  }

  if (req.method === 'GET' && req.url === '/health') {
    res.writeHead(200);
    res.end(JSON.stringify({ status: 'ok', is_building: isBuilding }));
    return;
  }

  res.writeHead(404);
  res.end(JSON.stringify({ status: 'error', message: 'Endpoint not found' }));
});

function triggerBuild() {
  if (isBuilding) return;
  isBuilding = true;
  console.log('⚡ Triggering static HTML re-build & prerender...');

  const startTime = Date.now();
  exec('npm run build', { cwd: path.resolve('.') }, (error, stdout, stderr) => {
    isBuilding = false;
    const duration = ((Date.now() - startTime) / 1000).toFixed(2);
    
    if (error) {
      console.error(`❌ Build failed after ${duration}s:`, error.message);
      if (stderr) console.error(stderr);
      return;
    }

    console.log(stdout);
    console.log(`✅ Static HTML re-build & prerender completed successfully in ${duration}s!`);
  });
}

server.listen(PORT, '0.0.0.0', () => {
  console.log(`📡 FE Builder HTTP Server listening at http://0.0.0.0:${PORT}`);
});
