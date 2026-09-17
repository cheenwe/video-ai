import type { Plugin } from 'vite'
import type { Plugin as EsbuildPlugin } from 'esbuild'

/**
 * Rewrite `import { IconX } from '@tabler/icons-vue'` to per-icon deep imports.
 * Avoids Vite optimizeDeps pre-bundling the entire ~4MB barrel file.
 */
function rewriteTablerImports(code: string): string | null {
  if (!/from\s*['"]@tabler\/icons-vue['"]/.test(code)) return null

  // Do not let trailing \s* swallow the newline before the next import.
  const importRe =
    /import\s*\{([^}]+)\}\s*from\s*['"]@tabler\/icons-vue['"][ \t]*;?/g

  let changed = false
  const next = code.replace(importRe, (_full, names: string) => {
    const lines = names
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
      .map((spec) => {
        const m = spec.match(/^(\w+)(?:\s+as\s+(\w+))?$/)
        if (!m) return `/* invalid tabler icon import: ${spec} */`
        const iconName = m[1]
        const localName = m[2] || m[1]
        changed = true
        return `import ${localName} from '@tabler/icons-vue/dist/esm/icons/${iconName}.mjs'`
      })
    return lines.join('\n')
  })

  return changed ? next : null
}

function tablerIconsEsbuildPlugin(): EsbuildPlugin {
  return {
    name: 'tabler-icons-deep-import-esbuild',
    setup(build) {
      build.onLoad({ filter: /\.(?:[cm]?[jt]sx?)$/ }, async (args) => {
        if (args.path.includes('node_modules')) return null
        const fs = await import('node:fs/promises')
        const source = await fs.readFile(args.path, 'utf8')
        const rewritten = rewriteTablerImports(source)
        if (!rewritten) return null
        return {
          contents: rewritten,
          loader: args.path.endsWith('tsx')
            ? 'tsx'
            : args.path.endsWith('jsx')
              ? 'jsx'
              : args.path.endsWith('ts')
                ? 'ts'
                : 'js',
        }
      })
    },
  }
}

export function tablerIconsDeepImport(): Plugin {
  return {
    name: 'tabler-icons-deep-import',
    enforce: 'pre',
    config() {
      return {
        optimizeDeps: {
          // Never pre-bundle the barrel; deep imports load individual icons.
          exclude: ['@tabler/icons-vue'],
          esbuildOptions: {
            plugins: [tablerIconsEsbuildPlugin()],
          },
        },
      }
    },
    transform(code, id) {
      if (id.includes('node_modules')) return null
      if (!/\.(?:vue|[cm]?[jt]sx?)(?:\?|$)/.test(id)) return null
      return rewriteTablerImports(code)
    },
  }
}
