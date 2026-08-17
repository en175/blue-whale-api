/**
 * Prebuild Script
 * Generates changelog before build
 */

import { generateChangelog } from './build-changelog';
import { generateDocsFromGateway } from './generate-docs-from-gateway';

async function prebuild() {
  console.log('═══════════════════════════════════════════════');
  console.log('🚀 Starting prebuild process...');
  console.log('═══════════════════════════════════════════════\n');

  const startTime = Date.now();

  try {
    await generateChangelog();
    await generateDocsFromGateway();

    const duration = ((Date.now() - startTime) / 1000).toFixed(2);

    console.log('═══════════════════════════════════════════════');
    console.log(`✅ Prebuild completed! Duration: ${duration}s`);
    console.log('═══════════════════════════════════════════════\n');
  } catch (error) {
    console.error('❌ Prebuild failed:', error);
    // Don't exit process, let the build continue
    console.log('⚠ Build will continue but may use old or missing data\n');
  }
}

// Execute prebuild
prebuild();
