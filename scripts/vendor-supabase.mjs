import {copyFile,mkdir} from 'node:fs/promises';
await mkdir(new URL('../assets/vendor/',import.meta.url),{recursive:true});
await copyFile(new URL('../node_modules/@supabase/supabase-js/dist/umd/supabase.js',import.meta.url),new URL('../assets/vendor/supabase.js',import.meta.url));
await copyFile(new URL('../node_modules/@supabase/supabase-js/LICENSE',import.meta.url),new URL('../assets/vendor/supabase-LICENSE.txt',import.meta.url));
console.log('Vendored the pinned Supabase browser SDK.');
