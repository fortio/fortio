// Behavioural test for the hidden-series permalink handling in fortio_chart.js.
// Runs the real script in a vm context with a minimal DOM and Chart stub.
// Usage: node ui/js_test/fortio_chart_hide_test.js [path/to/fortio_chart.js]

'use strict'

const assert = require('node:assert')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

const scriptPath = process.argv[2] || path.join(__dirname, '..', 'static', 'js', 'fortio_chart.js')
const source = fs.readFileSync(scriptPath, 'utf8')

// Loads the script in a fresh context whose document.location.search is `search`.
function load (search) {
  const env = { search, replaced: [] }
  const elements = {
    updtForm: {
      xmin: { value: '' },
      xmax: { value: '' },
      xlog: { checked: false },
      ymin: { value: '' },
      ymax: { value: '' },
      ylog: { checked: false }
    },
    chart1: { style: {}, getContext: () => ({}) },
    running: { style: {} },
    update: { style: {} },
    cc1: { style: {} }
  }
  function Chart (ctx, config) {
    this.config = config
    this.data = config.data
    this.options = config.options
    this.options.legend = this.options.legend || {}
  }
  Chart.prototype.update = function () {}
  Chart.prototype.destroy = function () {}
  Chart.defaults = { global: {} }
  const sandbox = {
    Chart,
    console,
    URLSearchParams,
    setTimeout,
    clearTimeout,
    document: {
      get location () {
        return { pathname: '/fortio/', search: env.search }
      },
      getElementById: (id) => elements[id] || null
    },
    window: {
      history: {
        replaceState: (state, title, url) => {
          env.replaced.push(url)
          env.search = url.slice(url.indexOf('?'))
        }
      }
    }
  }
  vm.createContext(sandbox)
  vm.runInContext(source, sandbox, { filename: scriptPath })
  return { ctx: sandbox, env }
}

// Evaluates an expression in the loaded script's context and returns a host-realm value.
function evalIn (ctx, expr) {
  return JSON.parse(vm.runInContext(`JSON.stringify(${expr})`, ctx))
}

function hiddenLabels (ctx) {
  return evalIn(ctx, 'mchart.data.datasets.filter(d => d.hidden).map(d => d.label)')
}

function hiddenInUrl (env) {
  return new URLSearchParams(env.search).getAll('hide')
}

function clickLegend (ctx, datasetIndex) {
  vm.runInContext(`mchart.options.legend.onClick(null, { datasetIndex: ${datasetIndex} })`, ctx)
}

// 1. Hidden series named in the URL are restored when the multi-run chart is built.
{
  const { ctx } = load('?hide=Max&hide=QPS')
  vm.runInContext('makeMultiChart()', ctx)
  assert.deepStrictEqual(hiddenLabels(ctx), ['Max', 'QPS'],
    'Restore hidden series from hide= parameters after reload')
  assert.strictEqual(evalIn(ctx, 'mchart.options.scales.yAxes[1].display'), false,
    'Restore hidden series: QPS axis must be hidden when QPS is in hide=')
}

// 2. With no hide= parameters, nothing is hidden and the QPS axis is shown.
{
  const { ctx } = load('?xMin=1')
  vm.runInContext('makeMultiChart()', ctx)
  assert.deepStrictEqual(hiddenLabels(ctx), [], 'No hide= parameters must hide nothing')
  assert.strictEqual(evalIn(ctx, 'mchart.options.scales.yAxes[1].display'), true,
    'No hide= parameters must show the QPS axis')
}

// 3. Toggling legend entries writes the hidden series to the permalink.
const first = load('')
vm.runInContext('makeMultiChart()', first.ctx)
clickLegend(first.ctx, 1) // Median
clickLegend(first.ctx, 8) // QPS
assert.deepStrictEqual(hiddenInUrl(first.env), ['Median', 'QPS'],
  'Legend toggles must write hidden series to the permalink (hide=)')

// 4. Toggling back removes the series from the permalink.
clickLegend(first.ctx, 1) // Median shown again
assert.deepStrictEqual(hiddenInUrl(first.env), ['QPS'],
  'Showing a series again must remove it from the permalink (hide=)')

// 5. A shared/reloaded permalink restores the same hidden set.
const reloaded = load(first.env.search)
vm.runInContext('makeMultiChart()', reloaded.ctx)
assert.deepStrictEqual(hiddenLabels(reloaded.ctx), ['QPS'],
  'Restore hidden series: permalink round trip must keep the same hidden set')

console.log('fortio_chart hide tests passed')
