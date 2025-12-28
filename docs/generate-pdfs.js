#!/usr/bin/env node

/**
 * Generate PDF documentation from Hugo site using Playwright
 *
 * This script builds the Hugo site and generates PDFs for each major section
 * as well as a complete documentation PDF.
 */

const { chromium } = require('playwright');
const { execSync } = require('child_process');
const fs = require('fs');
const path = require('path');

// Configuration
const OUTPUT_DIR = path.join('..', 'build', 'pdfs');
const BUILD_DIR = path.join('..', 'build', 'docs');
const SECTIONS = [
  { name: 'getting-started', title: 'Getting Started' },
  { name: 'concepts', title: 'Core Concepts' },
  { name: 'reference', title: 'Reference Documentation' },
  { name: 'operations', title: 'Operations Guide' },
  { name: 'community', title: 'Community' }
];

// Colors for console output
const colors = {
  blue: '\x1b[34m',
  green: '\x1b[32m',
  red: '\x1b[31m',
  reset: '\x1b[0m'
};

function log(message, color = 'reset') {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

async function buildHugoSite() {
  log('\n=== Building Hugo site ===', 'blue');
  try {
    execSync('hugo --quiet', { stdio: 'inherit', cwd: __dirname });
    log('✓ Hugo build complete', 'green');
  } catch (error) {
    log('✗ Hugo build failed', 'red');
    throw error;
  }
}

async function generatePDF(browser, section, outputFile) {
  const inputFile = path.join(BUILD_DIR, 'docs', section.name, 'index.html');

  // Check if input file exists
  if (!fs.existsSync(inputFile)) {
    log(`Warning: ${inputFile} not found, skipping...`, 'red');
    return;
  }

  log(`Generating ${outputFile}...`, 'blue');

  const page = await browser.newPage();

  try {
    // Load the HTML file
    await page.goto(`file://${path.resolve(inputFile)}`, {
      waitUntil: 'networkidle'
    });

    // Generate PDF with proper options
    await page.pdf({
      path: outputFile,
      format: 'A4',
      margin: {
        top: '20mm',
        bottom: '20mm',
        left: '15mm',
        right: '15mm'
      },
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: '<div></div>',
      footerTemplate: `
        <div style="font-size: 8px; width: 100%; text-align: center; padding: 5px;">
          Keystone Core Documentation - Page <span class="pageNumber"></span> of <span class="totalPages"></span>
        </div>
      `
    });

    const stats = fs.statSync(outputFile);
    const sizeMB = (stats.size / 1024 / 1024).toFixed(2);
    log(`✓ Generated ${outputFile} (${sizeMB} MB)`, 'green');
  } catch (error) {
    log(`✗ Failed to generate ${outputFile}: ${error.message}`, 'red');
  } finally {
    await page.close();
  }
}

async function generateCompletePDF(browser, outputFile) {
  const inputFile = path.join(BUILD_DIR, 'docs', 'index.html');

  if (!fs.existsSync(inputFile)) {
    log('Warning: Complete docs index not found, skipping...', 'red');
    return;
  }

  log(`\nGenerating complete documentation PDF...`, 'blue');

  const page = await browser.newPage();

  try {
    await page.goto(`file://${path.resolve(inputFile)}`, {
      waitUntil: 'networkidle'
    });

    await page.pdf({
      path: outputFile,
      format: 'A4',
      margin: {
        top: '20mm',
        bottom: '20mm',
        left: '15mm',
        right: '15mm'
      },
      printBackground: true,
      displayHeaderFooter: true,
      headerTemplate: '<div></div>',
      footerTemplate: `
        <div style="font-size: 8px; width: 100%; text-align: center; padding: 5px;">
          Keystone Core Documentation - Page <span class="pageNumber"></span> of <span class="totalPages"></span>
        </div>
      `,
      outline: true
    });

    const stats = fs.statSync(outputFile);
    const sizeMB = (stats.size / 1024 / 1024).toFixed(2);
    log(`✓ Generated ${outputFile} (${sizeMB} MB)`, 'green');
  } catch (error) {
    log(`✗ Failed to generate ${outputFile}: ${error.message}`, 'red');
  } finally {
    await page.close();
  }
}

async function main() {
  log('=== Keystone Core PDF Documentation Generator ===', 'blue');
  log('Using Playwright with Chromium\n', 'blue');

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

  // Launch browser
  log('\n=== Generating PDFs ===', 'blue');
  const browser = await chromium.launch();

  try {
    // Generate section PDFs
    for (const section of SECTIONS) {
      const outputFile = path.join(OUTPUT_DIR, `titananvil-${section.name}.pdf`);
      await generatePDF(browser, section, outputFile);
    }

    // Generate complete documentation PDF
    const completeOutputFile = path.join(OUTPUT_DIR, 'titananvil-complete.pdf');
    await generateCompletePDF(browser, completeOutputFile);

  } finally {
    await browser.close();
  }

  // Summary
  log('\n=== PDF Generation Complete ===', 'green');
  log(`Output directory: ${OUTPUT_DIR}/\n`, 'blue');

  if (fs.existsSync(OUTPUT_DIR)) {
    const files = fs.readdirSync(OUTPUT_DIR)
      .filter(f => f.endsWith('.pdf'))
      .map(f => {
        const stats = fs.statSync(path.join(OUTPUT_DIR, f));
        const sizeMB = (stats.size / 1024 / 1024).toFixed(2);
        return `  ${f} - ${sizeMB} MB`;
      });

    console.log(files.join('\n'));
  }

  log('\nDone!', 'green');
}

// Run the script
main().catch(error => {
  log(`\nFatal error: ${error.message}`, 'red');
  process.exit(1);
});
