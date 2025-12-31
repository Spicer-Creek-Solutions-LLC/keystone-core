#!/usr/bin/env node

/**
 * Generate PDF documentation from Hugo site using Playwright
 *
 * This script builds the Hugo site and generates PDFs with professional formatting.
 * It serves the docs via HTTP to ensure Mermaid diagrams and other assets load correctly.
 */

const { chromium } = require('playwright');
const { execSync, spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

// Configuration
const OUTPUT_DIR = path.join(__dirname, '..', 'build', 'pdfs');
const BUILD_DIR = path.join(__dirname, '..', 'build', 'docs');
const STATIC_DIR = path.join(__dirname, 'static');
const SERVER_PORT = 8765;

const SECTIONS = [
  { name: 'getting-started', title: 'Getting Started', weight: 1 },
  { name: 'concepts', title: 'Core Concepts', weight: 2 },
  { name: 'reference', title: 'Reference', weight: 3 },
  { name: 'operations', title: 'Operations Guide', weight: 4 },
  { name: 'community', title: 'Community', weight: 5 }
];

// Colors for console output
const colors = {
  blue: '\x1b[34m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  red: '\x1b[31m',
  dim: '\x1b[2m',
  reset: '\x1b[0m'
};

function log(message, color = 'reset') {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

async function buildHugoSite() {
  log('\n=== Building Hugo site ===', 'blue');
  try {
    execSync('hugo --quiet', { stdio: 'inherit', cwd: __dirname });
    log('Hugo build complete', 'green');
  } catch (error) {
    log('Hugo build failed', 'red');
    throw error;
  }
}

/**
 * Start a simple HTTP server to serve the built docs
 * This is required for Mermaid diagrams to load from CDN
 */
function startServer() {
  return new Promise((resolve, reject) => {
    const server = http.createServer((req, res) => {
      let filePath = path.join(BUILD_DIR, req.url === '/' ? 'index.html' : req.url);

      // Handle directory requests
      if (fs.existsSync(filePath) && fs.statSync(filePath).isDirectory()) {
        filePath = path.join(filePath, 'index.html');
      }

      // Determine content type
      const ext = path.extname(filePath).toLowerCase();
      const contentTypes = {
        '.html': 'text/html',
        '.css': 'text/css',
        '.js': 'application/javascript',
        '.json': 'application/json',
        '.png': 'image/png',
        '.jpg': 'image/jpeg',
        '.gif': 'image/gif',
        '.svg': 'image/svg+xml',
        '.woff': 'font/woff',
        '.woff2': 'font/woff2',
        '.ttf': 'font/ttf',
        '.eot': 'application/vnd.ms-fontobject'
      };
      const contentType = contentTypes[ext] || 'application/octet-stream';

      fs.readFile(filePath, (err, content) => {
        if (err) {
          if (err.code === 'ENOENT') {
            res.writeHead(404);
            res.end('Not found');
          } else {
            res.writeHead(500);
            res.end('Server error');
          }
        } else {
          res.writeHead(200, { 'Content-Type': contentType });
          res.end(content);
        }
      });
    });

    server.listen(SERVER_PORT, '127.0.0.1', () => {
      log(`  HTTP server started on port ${SERVER_PORT}`, 'dim');
      resolve(server);
    });

    server.on('error', reject);
  });
}

/**
 * Get the print CSS content
 */
function getPrintCSS() {
  const printCSSPath = path.join(STATIC_DIR, 'css', 'print.css');
  if (fs.existsSync(printCSSPath)) {
    return fs.readFileSync(printCSSPath, 'utf-8');
  }
  log('Warning: print.css not found, using defaults', 'yellow');
  return getDefaultPrintCSS();
}

/**
 * Default print CSS if custom file not found
 */
function getDefaultPrintCSS() {
  return `
    @page {
      size: A4;
      margin: 25mm 20mm 30mm 20mm;
    }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 11pt;
      line-height: 1.5;
      color: #333;
    }

    /* Hide navigation elements */
    nav, .td-navbar, .td-sidebar, .td-toc, footer,
    .edit-page-block, .feedback-block, .page-meta-links,
    button, .btn, .search-form, #search {
      display: none !important;
    }

    /* Full width content */
    .td-main main, .td-content, article, .container {
      max-width: 100% !important;
      width: 100% !important;
      padding: 0 !important;
      margin: 0 !important;
    }

    /* Page breaks */
    h1 { page-break-before: always; }
    h1:first-child { page-break-before: avoid; }
    h2, h3 { page-break-after: avoid; }
    pre, table, figure, .mermaid { page-break-inside: avoid; }

    /* Code blocks */
    pre {
      background-color: #f5f5f5 !important;
      border: 1px solid #ddd !important;
      padding: 10pt !important;
      font-size: 9pt !important;
      overflow-x: auto !important;
    }

    /* Mermaid diagrams */
    .mermaid svg {
      max-width: 100% !important;
      height: auto !important;
    }

    /* Links */
    a { color: #2563eb; text-decoration: none; }
    a[href^="http"]:after {
      content: " (" attr(href) ")";
      font-size: 8pt;
      color: #666;
    }
    a[href^="#"]:after, a[href^="/"]:after { content: none; }
  `;
}

/**
 * Wait for Mermaid diagrams to render
 */
async function waitForMermaid(page) {
  try {
    // Wait for Mermaid to initialize and render diagrams
    await page.waitForFunction(() => {
      const mermaidElements = document.querySelectorAll('.mermaid');
      if (mermaidElements.length === 0) return true; // No diagrams, continue

      // Check if all mermaid elements have been rendered (contain SVG)
      for (const el of mermaidElements) {
        if (!el.querySelector('svg') && !el.getAttribute('data-processed')) {
          return false;
        }
      }
      return true;
    }, { timeout: 15000 });

    // Extra wait for SVG rendering to complete
    await page.waitForTimeout(1000);
  } catch (e) {
    log('  Warning: Mermaid render timeout, some diagrams may be missing', 'yellow');
  }
}

/**
 * Generate PDF from a URL
 */
async function generatePDF(browser, inputUrl, outputFile, title) {
  log(`  Generating: ${path.basename(outputFile)}`, 'dim');

  const page = await browser.newPage();

  try {
    // Navigate to the page
    await page.goto(inputUrl, {
      waitUntil: 'networkidle',
      timeout: 30000
    });

    // Wait for Mermaid diagrams to render
    await waitForMermaid(page);

    // Inject print CSS
    const printCSS = getPrintCSS();
    await page.addStyleTag({ content: printCSS });

    // Wait for styles to apply
    await page.waitForTimeout(500);

    // Generate PDF
    await page.pdf({
      path: outputFile,
      format: 'A4',
      margin: {
        top: '25mm',
        bottom: '30mm',
        left: '20mm',
        right: '20mm'
      },
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: `
        <div style="font-size: 9px; width: 100%; text-align: center; color: #666; padding: 5px 20px;">
          ${title}
        </div>
      `,
      footerTemplate: `
        <div style="font-size: 9px; width: 100%; display: flex; justify-content: space-between; padding: 5px 20px; color: #666;">
          <span>Keystone Core Documentation</span>
          <span>Page <span class="pageNumber"></span> of <span class="totalPages"></span></span>
        </div>
      `
    });

    if (fs.existsSync(outputFile)) {
      const stats = fs.statSync(outputFile);
      log(`  Generated: ${formatSize(stats.size)}`, 'green');
      return { file: outputFile, size: stats.size };
    }
  } catch (error) {
    log(`  Error: ${error.message}`, 'red');
  } finally {
    await page.close();
  }

  return null;
}

/**
 * Find all pages in a section by scanning the build directory
 */
function findSectionPages(sectionName) {
  const sectionDir = path.join(BUILD_DIR, 'docs', sectionName);
  const pages = [];

  function scanDir(dir, prefix = '') {
    if (!fs.existsSync(dir)) return;

    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.isDirectory()) {
        const indexPath = path.join(dir, entry.name, 'index.html');
        if (fs.existsSync(indexPath)) {
          pages.push({
            path: `${prefix}${entry.name}/`,
            name: entry.name
          });
        }
        // Recursively scan subdirectories
        scanDir(path.join(dir, entry.name), `${prefix}${entry.name}/`);
      }
    }
  }

  // Add the section index page first
  const indexPath = path.join(sectionDir, 'index.html');
  if (fs.existsSync(indexPath)) {
    pages.push({ path: '', name: '_index' });
  }

  scanDir(sectionDir);

  // Sort pages to maintain logical order (index first, then alphabetical)
  return pages.sort((a, b) => {
    if (a.name === '_index') return -1;
    if (b.name === '_index') return 1;
    return a.path.localeCompare(b.path);
  });
}

/**
 * Generate section PDF by combining all sub-pages
 */
async function generateSectionPDF(browser, section, baseUrl) {
  const outputFile = path.join(OUTPUT_DIR, `keystone-core-${section.name}.pdf`);
  log(`\nGenerating ${section.title}...`, 'blue');

  const pages = findSectionPages(section.name);
  log(`  Found ${pages.length} pages in section`, 'dim');

  if (pages.length === 0) {
    log(`  Warning: No pages found for section ${section.name}`, 'yellow');
    return null;
  }

  const page = await browser.newPage();

  try {
    // Collect content from all pages
    let combinedHTML = `
      <!DOCTYPE html>
      <html>
      <head>
        <meta charset="UTF-8">
        <title>${section.title} - Keystone Core Documentation</title>
        <style>
          body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
          .page-break { page-break-before: always; }
          .section-content { margin-bottom: 2em; }
        </style>
      </head>
      <body>
    `;

    for (let i = 0; i < pages.length; i++) {
      const pageInfo = pages[i];
      const pageUrl = `${baseUrl}/docs/${section.name}/${pageInfo.path}`;

      log(`    Loading: ${pageInfo.path || 'index'}`, 'dim');

      await page.goto(pageUrl, { waitUntil: 'networkidle', timeout: 30000 });
      await waitForMermaid(page);

      // Extract the main content
      const content = await page.evaluate(() => {
        // Find the main content area (Docsy uses .td-content)
        const mainContent = document.querySelector('.td-content') ||
                           document.querySelector('main article') ||
                           document.querySelector('main');
        if (!mainContent) return '';

        // Clone to avoid modifying the original
        const clone = mainContent.cloneNode(true);

        // Remove navigation elements
        const toRemove = clone.querySelectorAll('.td-toc, .edit-page-block, .feedback-block, .page-meta-links, nav, .breadcrumb');
        toRemove.forEach(el => el.remove());

        return clone.innerHTML;
      });

      if (content) {
        const pageBreak = i > 0 ? ' page-break' : '';
        combinedHTML += `<div class="section-content${pageBreak}">${content}</div>\n`;
      }
    }

    combinedHTML += '</body></html>';

    // Create a new page with combined content
    await page.setContent(combinedHTML, { waitUntil: 'networkidle' });

    // Wait for any Mermaid diagrams to render in combined content
    await waitForMermaid(page);

    // Inject print CSS
    const printCSS = getPrintCSS();
    await page.addStyleTag({ content: printCSS });
    await page.waitForTimeout(500);

    // Generate PDF
    await page.pdf({
      path: outputFile,
      format: 'A4',
      margin: { top: '25mm', bottom: '30mm', left: '20mm', right: '20mm' },
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: `
        <div style="font-size: 9px; width: 100%; text-align: center; color: #666; padding: 5px 20px;">
          ${section.title}
        </div>
      `,
      footerTemplate: `
        <div style="font-size: 9px; width: 100%; display: flex; justify-content: space-between; padding: 5px 20px; color: #666;">
          <span>Keystone Core Documentation</span>
          <span>Page <span class="pageNumber"></span> of <span class="totalPages"></span></span>
        </div>
      `
    });

    if (fs.existsSync(outputFile)) {
      const stats = fs.statSync(outputFile);
      log(`  Generated: ${formatSize(stats.size)}`, 'green');
      return { file: outputFile, size: stats.size };
    }
  } catch (error) {
    log(`  Error: ${error.message}`, 'red');
  } finally {
    await page.close();
  }

  return null;
}

/**
 * Generate complete documentation PDF by combining all sections
 */
async function generateCompletePDF(browser, baseUrl) {
  const outputFile = path.join(OUTPUT_DIR, 'keystone-core-complete.pdf');
  log('\nGenerating complete documentation...', 'blue');

  const page = await browser.newPage();

  try {
    let combinedHTML = `
      <!DOCTYPE html>
      <html>
      <head>
        <meta charset="UTF-8">
        <title>Keystone Core - Complete Documentation</title>
        <style>
          body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
          .page-break { page-break-before: always; }
          .section-content { margin-bottom: 2em; }
          .section-title { font-size: 2em; font-weight: bold; margin-bottom: 1em; color: #2563eb; }
        </style>
      </head>
      <body>
    `;

    let isFirstPage = true;

    for (const section of SECTIONS) {
      const pages = findSectionPages(section.name);
      log(`  Processing ${section.title} (${pages.length} pages)...`, 'dim');

      for (const pageInfo of pages) {
        const pageUrl = `${baseUrl}/docs/${section.name}/${pageInfo.path}`;

        await page.goto(pageUrl, { waitUntil: 'networkidle', timeout: 30000 });
        await waitForMermaid(page);

        const content = await page.evaluate(() => {
          const mainContent = document.querySelector('.td-content') ||
                             document.querySelector('main article') ||
                             document.querySelector('main');
          if (!mainContent) return '';

          const clone = mainContent.cloneNode(true);
          const toRemove = clone.querySelectorAll('.td-toc, .edit-page-block, .feedback-block, .page-meta-links, nav, .breadcrumb');
          toRemove.forEach(el => el.remove());

          return clone.innerHTML;
        });

        if (content) {
          const pageBreak = !isFirstPage ? ' page-break' : '';
          combinedHTML += `<div class="section-content${pageBreak}">${content}</div>\n`;
          isFirstPage = false;
        }
      }
    }

    combinedHTML += '</body></html>';

    await page.setContent(combinedHTML, { waitUntil: 'networkidle' });
    await waitForMermaid(page);

    const printCSS = getPrintCSS();
    await page.addStyleTag({ content: printCSS });
    await page.waitForTimeout(500);

    await page.pdf({
      path: outputFile,
      format: 'A4',
      margin: { top: '25mm', bottom: '30mm', left: '20mm', right: '20mm' },
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: `
        <div style="font-size: 9px; width: 100%; text-align: center; color: #666; padding: 5px 20px;">
          Keystone Core Documentation
        </div>
      `,
      footerTemplate: `
        <div style="font-size: 9px; width: 100%; display: flex; justify-content: space-between; padding: 5px 20px; color: #666;">
          <span>Keystone Core Documentation</span>
          <span>Page <span class="pageNumber"></span> of <span class="totalPages"></span></span>
        </div>
      `
    });

    if (fs.existsSync(outputFile)) {
      const stats = fs.statSync(outputFile);
      log(`  Generated: ${formatSize(stats.size)}`, 'green');
      return { file: outputFile, size: stats.size };
    }
  } catch (error) {
    log(`  Error: ${error.message}`, 'red');
  } finally {
    await page.close();
  }

  return null;
}

async function main() {
  const startTime = Date.now();

  log('=== Keystone Core PDF Documentation Generator ===', 'blue');
  log('Using Playwright with HTTP server for full diagram support\n', 'dim');

  // Parse command line arguments
  const args = process.argv.slice(2);
  const sectionOnly = args.find(a => a.startsWith('--section='))?.split('=')[1];

  // Create output directory
  if (!fs.existsSync(OUTPUT_DIR)) {
    fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  }

  // Build Hugo site
  await buildHugoSite();

  // Check if build directory exists
  if (!fs.existsSync(BUILD_DIR)) {
    log(`\nError: Hugo build directory ${BUILD_DIR} not found`, 'red');
    process.exit(1);
  }

  // Start HTTP server
  log('\n=== Starting HTTP server ===', 'blue');
  const server = await startServer();
  const baseUrl = `http://127.0.0.1:${SERVER_PORT}`;

  // Launch browser
  log('\n=== Generating PDFs ===', 'blue');
  const browser = await chromium.launch({
    headless: true
  });

  const results = [];

  try {
    if (sectionOnly) {
      // Generate single section
      const section = SECTIONS.find(s => s.name === sectionOnly);
      if (section) {
        const result = await generateSectionPDF(browser, section, baseUrl);
        if (result) results.push(result);
      } else {
        log(`Unknown section: ${sectionOnly}`, 'red');
        log(`Available sections: ${SECTIONS.map(s => s.name).join(', ')}`, 'dim');
      }
    } else {
      // Generate all section PDFs
      for (const section of SECTIONS) {
        const result = await generateSectionPDF(browser, section, baseUrl);
        if (result) results.push(result);
      }

      // Generate complete PDF
      const result = await generateCompletePDF(browser, baseUrl);
      if (result) results.push(result);
    }
  } finally {
    await browser.close();
    server.close();
    log('\n  HTTP server stopped', 'dim');
  }

  // Summary
  const elapsed = ((Date.now() - startTime) / 1000).toFixed(1);
  const totalSize = results.reduce((sum, r) => sum + r.size, 0);

  log('\n=== PDF Generation Complete ===', 'green');
  log(`Output directory: ${OUTPUT_DIR}/`, 'dim');
  log(`Generated ${results.length} PDFs (${formatSize(totalSize)}) in ${elapsed}s\n`, 'dim');

  // List generated files
  if (fs.existsSync(OUTPUT_DIR)) {
    const files = fs.readdirSync(OUTPUT_DIR)
      .filter(f => f.endsWith('.pdf'))
      .sort()
      .map(f => {
        const stats = fs.statSync(path.join(OUTPUT_DIR, f));
        return `  ${f.padEnd(40)} ${formatSize(stats.size).padStart(10)}`;
      });

    console.log(files.join('\n'));
  }

  log('\nDone!', 'green');
}

// Run the script
main().catch(error => {
  log(`\nFatal error: ${error.message}`, 'red');
  console.error(error);
  process.exit(1);
});
