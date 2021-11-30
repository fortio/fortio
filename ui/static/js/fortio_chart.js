// Copyright 2017 Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// TODO: object-ify

const linearXAxe = {
  type: 'linear',
  scaleLabel: {
    display: true,
    labelString: 'Response time in ms',
    ticks: {
      min: 0,
      beginAtZero: true
    }
  }
}
const logXAxe = {
  type: 'logarithmic',
  scaleLabel: {
    display: true,
    labelString: 'Response time in ms (log scale)'
  },
  ticks: {
    // min: dataH[0].x, // newer chart.js are ok with 0 on x axis too
    callback: function (tick, index, ticks) {
      return tick.toLocaleString()
    }
  }
}
const linearYAxe = {
  id: 'H',
  type: 'linear',
  ticks: {
    beginAtZero: true
  },
  scaleLabel: {
    display: true,
    labelString: 'Count'
  }
}
const logYAxe = {
  id: 'H',
  type: 'logarithmic',
  display: true,
  ticks: {
    // min: 1, // log mode works even with 0s
    // Needed to not get scientific notation display:
    callback: function (tick, index, ticks) {
      return tick.toString()
    }
  },
  scaleLabel: {
    display: true,
    labelString: 'Count (log scale)'
  }
}

let chart = {}
let overlayChart = {}
let mchart = {}

function myRound (v, digits = 6) {
  const p = Math.pow(10, digits)
  return Math.round(v * p) / p
}

function pad (n) {
  return (n < 10) ? ('0' + n) : n
}

function formatDate (dStr) {
  const d = new Date(dStr)
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' +
        pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds())
}

function makeTitle (res) {
  const title = []
  let firstLine = ''
  if ((typeof res.RunID !== 'undefined')  &&  (res.RunID!== 0)) {
    firstLine = '(' + res.RunID + ') '
  }
  if (res.Labels !== '') {
    firstLine += res.Labels + ' - '
  }
  if (res.URL) { // http results
    firstLine+= res.URL
  } else { // grpc/tcp results
    firstLine+= res.Destination
  }
  title.push(firstLine+ ' - ' + formatDate(res.StartTime))
  let percStr = 'min ' + myRound(1000.0 * res.DurationHistogram.Min, 3) + ' ms, average ' + myRound(1000.0 * res.DurationHistogram.Avg, 3) + ' ms'
  if (res.DurationHistogram.Percentiles) {
    for (let i = 0; i < res.DurationHistogram.Percentiles.length; i++) {
      const p = res.DurationHistogram.Percentiles[i]
      percStr += ', p' + p.Percentile + ' ' + myRound(1000 * p.Value, 2) + ' ms'
    }
  }
  percStr += ', max ' + myRound(1000.0 * res.DurationHistogram.Max, 3) + ' ms'
  let statusOk = res.RetCodes[200]
  if (!statusOk) { // grpc or tcp results
    statusOk = res.RetCodes.SERVING || res.RetCodes.OK
  }
  const total = res.DurationHistogram.Count
  let errStr = 'no error'
  if (statusOk !== total) {
    if (statusOk) {
      errStr = myRound(100.0 * (total - statusOk) / total, 2) + '% errors'
    } else {
      errStr = '100% errors!'
    }
  }
  title.push('Response time histogram at ' + res.RequestedQPS + ' target qps (' +
        myRound(res.ActualQPS, 1) + ' actual) ' + res.NumThreads + ' connections for ' +
        res.RequestedDuration + ' (actual time ' + myRound(res.ActualDuration / 1e9, 1) + 's), jitter: ' +
  res.Jitter + ', ' + errStr)
  title.push(percStr)
  return title
}

function fortioResultToJsChartData (res) {
  const dataP = [{
    x: 0.0,
    y: 0.0
  }]
  const len = res.DurationHistogram.Data.length
  let prevX = 0.0
  let prevY = 0.0
  for (let i = 0; i < len; i++) {
    const it = res.DurationHistogram.Data[i]
    let x = myRound(1000.0 * it.Start)
    if (i === 0) {
      // Extra point, 1/N at min itself
      dataP.push({
        x: x,
        y: myRound(100.0 / res.DurationHistogram.Count, 3)
      })
    } else {
      if (prevX !== x) {
        dataP.push({
          x: x,
          y: prevY
        })
      }
    }
    x = myRound(1000.0 * it.End)
    const y = myRound(it.Percent, 3)
    dataP.push({
      x: x,
      y: y
    })
    prevX = x
    prevY = y
  }
  const dataH = []
  let prev = 1000.0 * res.DurationHistogram.Data[0].Start
  for (let i = 0; i < len; i++) {
    const it = res.DurationHistogram.Data[i]
    const startX = 1000.0 * it.Start
    const endX = 1000.0 * it.End
    if (startX !== prev) {
      dataH.push({
        x: myRound(prev),
        y: 0
      }, {
        x: myRound(startX),
        y: 0
      })
    }
    dataH.push({
      x: myRound(startX),
      y: it.Count
    }, {
      x: myRound(endX),
      y: it.Count
    })
    prev = endX
  }
  return {
    title: makeTitle(res),
    dataP: dataP,
    dataH: dataH
  }
}

function showChart (data) {
  makeChart(data)
  // Load configuration (min, max, isLogarithmic, ...) from the update form.
  updateChartOptions(chart)
  toggleVisibility()
}

function toggleVisibility () {
  document.getElementById('running').style.display = 'none'
  document.getElementById('cc1').style.display = 'block'
  document.getElementById('update').style.visibility = 'visible'
}

function makeOverlayChartTitle (titleA, titleB) {
  // Each string in the array is a separate line
  return [
    'A: ' + titleA[0], titleA[1], // Skip 3rd line.
    '',
    'B: ' + titleB[0], titleB[1] // Skip 3rd line.
  ]
}

function makeOverlayChart (dataA, dataB) {
  const chartEl = document.getElementById('chart1')
  chartEl.style.visibility = 'visible'
  if (Object.keys(overlayChart).length !== 0) {
    return
  }
  deleteSingleChart()
  deleteMultiChart()
  const ctx = chartEl.getContext('2d')
  const title = makeOverlayChartTitle(dataA.title, dataB.title)
  overlayChart = new Chart(ctx, {
    type: 'line',
    data: {
      // "Cumulative %" datasets are listed first so they are drawn on top of the histograms.
      datasets: [{
        label: 'A: Cumulative %',
        data: dataA.dataP,
        fill: false,
        yAxisID: 'P',
        stepped: true,
        backgroundColor: 'rgba(134, 87, 167, 1)',
        borderColor: 'rgba(134, 87, 167, 1)',
        cubicInterpolationMode: 'monotone'
      }, {
        label: 'B: Cumulative %',
        data: dataB.dataP,
        fill: false,
        yAxisID: 'P',
        stepped: true,
        backgroundColor: 'rgba(204, 102, 0)',
        borderColor: 'rgba(204, 102, 0)',
        cubicInterpolationMode: 'monotone'
      }, {
        label: 'A: Histogram: Count',
        data: dataA.dataH,
        yAxisID: 'H',
        pointStyle: 'rect',
        radius: 1,
        borderColor: 'rgba(87, 167, 134, .9)',
        backgroundColor: 'rgba(87, 167, 134, .75)',
        lineTension: 0
      }, {
        label: 'B: Histogram: Count',
        data: dataB.dataH,
        yAxisID: 'H',
        pointStyle: 'rect',
        radius: 1,
        borderColor: 'rgba(36, 64, 238, .9)',
        backgroundColor: 'rgba(36, 64, 238, .75)',
        lineTension: 0
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      title: {
        display: true,
        fontStyle: 'normal',
        text: title
      },
      scales: {
        xAxes: [
          linearXAxe
        ],
        yAxes: [{
          id: 'P',
          position: 'right',
          ticks: {
            beginAtZero: true,
            max: 100
          },
          scaleLabel: {
            display: true,
            labelString: '%'
          }
        },
        linearYAxe
        ]
      }
    }
  })
  updateChart(overlayChart)
}

function makeChart (data) {
  const chartEl = document.getElementById('chart1')
  chartEl.style.visibility = 'visible'
  if (Object.keys(chart).length === 0) {
    deleteOverlayChart()
    deleteMultiChart()
    // Creation (first or switch) time
    const ctx = chartEl.getContext('2d')
    chart = new Chart(ctx, {
      type: 'line',
      data: {
        datasets: [{
          label: 'Cumulative %',
          data: data.dataP,
          fill: false,
          yAxisID: 'P',
          stepped: true,
          backgroundColor: 'rgba(134, 87, 167, 1)',
          borderColor: 'rgba(134, 87, 167, 1)',
          cubicInterpolationMode: 'monotone'
        },
        {
          label: 'Histogram: Count',
          data: data.dataH,
          yAxisID: 'H',
          pointStyle: 'rect',
          radius: 1,
          borderColor: 'rgba(87, 167, 134, .9)',
          backgroundColor: 'rgba(87, 167, 134, .75)',
          lineTension: 0
        }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        title: {
          display: true,
          fontStyle: 'normal',
          text: data.title
        },
        scales: {
          xAxes: [
            linearXAxe
          ],
          yAxes: [{
            id: 'P',
            position: 'right',
            ticks: {
              beginAtZero: true,
              max: 100
            },
            scaleLabel: {
              display: true,
              labelString: '%'
            }
          },
          linearYAxe
          ]
        }
      }
    })
    // TODO may need updateChart() if we persist settings even the first time
  } else {
    chart.data.datasets[0].data = data.dataP
    chart.data.datasets[1].data = data.dataH
    chart.options.title.text = data.title
    updateChart(chart)
  }
}

function getUpdateForm () {
  const form = document.getElementById('updtForm')
  const xMin = form.xmin.value.trim()
  const xMax = form.xmax.value.trim()
  const xIsLogarithmic = form.xlog.checked
  const yMin = form.ymin.value.trim()
  const yMax = form.ymax.value.trim()
  const yIsLogarithmic = form.ylog.checked
  return { xMin, xMax, xIsLogarithmic, yMin, yMax, yIsLogarithmic }
}

function getSelectedResults () {
  // Undefined if on "graph-only" page
  const select = document.getElementById('files')
  let selectedResults
  if (select) {
    const selectedOptions = select.selectedOptions
    selectedResults = []
    for (const option of selectedOptions) {
      selectedResults.push(option.text)
    }
  } else {
    selectedResults = undefined
  }
  return selectedResults
}

function updateQueryString () {
  const location = document.location
  const params = new URLSearchParams(location.search)
  const form = getUpdateForm()
  params.set('xMin', form.xMin)
  params.set('xMax', form.xMax)
  params.set('xLog', form.xIsLogarithmic)
  params.set('yMin', form.yMin)
  params.set('yMax', form.yMax)
  params.set('yLog', form.yIsLogarithmic)
  const selectedResults = getSelectedResults()
  params.delete('sel')
  if (selectedResults) {
    for (const result of selectedResults) {
      params.append('sel', result)
    }
  }
  window.history.replaceState({}, '', `${location.pathname}?${params}`)
}

function updateChartOptions (chart) {
  const form = getUpdateForm()
  const scales = chart.config.options.scales
  const newXMin = parseFloat(form.xMin)
  const newYMin = parseFloat(form.yMin)
  const newXAxis = form.xIsLogarithmic ? logXAxe : linearXAxe
  const newYAxis = form.yIsLogarithmic ? logYAxe : linearYAxe
  chart.config.options.scales = {
    xAxes: [newXAxis],
    yAxes: [scales.yAxes[0], newYAxis]
  }
  chart.update() // needed for scales.xAxes[0] to exist
  const newNewXAxis = chart.config.options.scales.xAxes[0]
  newNewXAxis.ticks.min = form.xMin === '' ? undefined : newXMin
  const formXMax = form.xMax
  newNewXAxis.ticks.max = formXMax === '' || formXMax === 'max' ?
      undefined :
      parseFloat(formXMax)
  const newNewYAxis = chart.config.options.scales.yAxes[1]
  newNewYAxis.ticks.min = form.yMin === '' ? undefined : newYMin
  const formYMax = form.yMax
  newNewYAxis.ticks.max = formYMax === '' || formYMax === 'max' ?
      undefined :
      parseFloat(formYMax)
  chart.update()
}

function objHasProps (obj) {
  return Object.keys(obj).length > 0
}

function getCurrentChart () {
  let currentChart
  if (objHasProps(chart)) {
    currentChart = chart
  } else if (objHasProps(overlayChart)) {
    currentChart = overlayChart
  } else if (objHasProps(mchart)) {
    currentChart = mchart
  } else {
    currentChart = undefined
  }
  return currentChart
}

let timeoutID = 0
function updateChart (chart = getCurrentChart()) {
  updateChartOptions(chart)
  if (timeoutID > 0) {
    clearTimeout(timeoutID)
  }
  timeoutID = setTimeout(updateQueryString, 750)
}

function multiLabel (res) {
  let l = formatDate(res.StartTime)
  if (res.Labels !== '') {
    l += ' - ' + res.Labels
  }
  return l
}

function findData (slot, idx, res, p) {
  // Not very efficient but there are only a handful of percentiles
  const pA = res.DurationHistogram.Percentiles
  if (!pA) {
    //    console.log('No percentiles in res', res)
    return
  }
  const pN = Number(p)
  for (let i = 0; i < pA.length; i++) {
    if (pA[i].Percentile === pN) {
      mchart.data.datasets[slot].data[idx] = 1000.0 * pA[i].Value
      return
    }
  }
  console.log('Not Found', p, pN, pA)
  // not found, not set
}

function fortioAddToMultiResult (i, res) {
  mchart.data.labels[i] = multiLabel(res)
  mchart.data.datasets[0].data[i] = 1000.0 * res.DurationHistogram.Min
  findData(1, i, res, '50')
  mchart.data.datasets[2].data[i] = 1000.0 * res.DurationHistogram.Avg
  findData(3, i, res, '75')
  findData(4, i, res, '90')
  findData(5, i, res, '99')
  findData(6, i, res, '99.9')
  mchart.data.datasets[7].data[i] = 1000.0 * res.DurationHistogram.Max
  mchart.data.datasets[8].data[i] = res.ActualQPS
}

function endMultiChart (len) {
  mchart.data.labels = mchart.data.labels.slice(0, len)
  for (let i = 0; i < mchart.data.datasets.length; i++) {
    mchart.data.datasets[i].data = mchart.data.datasets[i].data.slice(0, len)
  }
  mchart.update()
}

function deleteOverlayChart () {
  if (Object.keys(overlayChart).length === 0) {
    return
  }
  overlayChart.destroy()
  overlayChart = {}
}

function deleteMultiChart () {
  if (Object.keys(mchart).length === 0) {
    return
  }
  mchart.destroy()
  mchart = {}
}

function deleteSingleChart () {
  if (Object.keys(chart).length === 0) {
    return
  }
  chart.destroy()
  chart = {}
}

function makeMultiChart () {
  document.getElementById('running').style.display = 'none'
  document.getElementById('update').style.visibility = 'hidden'
  const chartEl = document.getElementById('chart1')
  chartEl.style.visibility = 'visible'
  if (Object.keys(mchart).length !== 0) {
    return
  }
  deleteSingleChart()
  deleteOverlayChart()
  const ctx = chartEl.getContext('2d')
  mchart = new Chart(ctx, {
    type: 'line',
    data: {
      labels: [],
      datasets: [
        {
          label: 'Min',
          fill: false,
          stepped: true,
          borderColor: 'hsla(111, 100%, 40%, .8)',
          backgroundColor: 'hsla(111, 100%, 40%, .8)'
        },
        {
          label: 'Median',
          fill: false,
          stepped: true,
          borderDash: [5, 5],
          borderColor: 'hsla(220, 100%, 40%, .8)',
          backgroundColor: 'hsla(220, 100%, 40%, .8)'
        },
        {
          label: 'Avg',
          fill: false,
          stepped: true,
          backgroundColor: 'hsla(266, 100%, 40%, .8)',
          borderColor: 'hsla(266, 100%, 40%, .8)'
        },
        {
          label: 'p75',
          fill: false,
          stepped: true,
          backgroundColor: 'hsla(60, 100%, 40%, .8)',
          borderColor: 'hsla(60, 100%, 40%, .8)'
        },
        {
          label: 'p90',
          fill: false,
          stepped: true,
          backgroundColor: 'hsla(45, 100%, 40%, .8)',
          borderColor: 'hsla(45, 100%, 40%, .8)'
        },
        {
          label: 'p99',
          fill: false,
          stepped: true,
          backgroundColor: 'hsla(30, 100%, 40%, .8)',
          borderColor: 'hsla(30, 100%, 40%, .8)'
        },
        {
          label: 'p99.9',
          fill: false,
          stepped: true,
          backgroundColor: 'hsla(15, 100%, 40%, .8)',
          borderColor: 'hsla(15, 100%, 40%, .8)'
        },
        {
          label: 'Max',
          fill: false,
          stepped: true,
          borderColor: 'hsla(0, 100%, 40%, .8)',
          backgroundColor: 'hsla(0, 100%, 40%, .8)'
        },
        {
          label: 'QPS',
          yAxisID: 'qps',
          fill: false,
          stepped: true,
          borderColor: 'rgba(0, 0, 0, .8)',
          backgroundColor: 'rgba(0, 0, 0, .8)'
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      title: {
        display: true,
        fontStyle: 'normal',
        text: ['Latency in milliseconds']
      },
      elements: {
        line: {
          tension: 0 // disables bezier curves
        }
      },
      scales: {
        yAxes: [{
          id: 'ms',
          ticks: {
            beginAtZero: true
          },
          scaleLabel: {
            display: true,
            labelString: 'ms'
          }
        }, {
          id: 'qps',
          position: 'right',
          ticks: {
            beginAtZero: true
          },
          scaleLabel: {
            display: true,
            labelString: 'QPS'
          }
        }]
      }
    }
  })
  // Hide QPS axis on clicking QPS dataset.
  mchart.options.legend.onClick = (event, legendItem) => {
    // Toggle dataset hidden (default behavior).
    const dataset = mchart.data.datasets[legendItem.datasetIndex]
    dataset.hidden = !dataset.hidden
    if (dataset.label === 'QPS') {
      // Toggle QPS y-axis.
      const qpsYAxis = mchart.options.scales.yAxes[1]
      qpsYAxis.display = !qpsYAxis.display
    }
    mchart.update()
  }
}

function runTestForDuration (durationInSeconds) {
  const progressBar = document.getElementById('progressBar')
  if (durationInSeconds <= 0) {
    // infinite case
    progressBar.removeAttribute('value')
    return
  }
  const startTimeMillis = Date.now()
  const updatePercentage = function () {
    const barPercentage = Math.min(100, (Date.now() - startTimeMillis) / (10 * durationInSeconds))
    progressBar.value = barPercentage
    if (barPercentage < 100) {
      setTimeout(updatePercentage, 50 /* milliseconds */) // 20fps
    }
  }
  updatePercentage()
}

let lastDuration = ''

function toggleDuration (el) {
  const d = document.getElementById('duration')
  if (el.checked) {
    lastDuration = d.value
    d.value = ''
  } else {
    d.value = lastDuration
  }
}

const customHeaderElement = '<input type="text" name="H" size=40 value="" /> <br />'

function addCustomHeader () {
  const customHeaderElements = document.getElementsByName('H')
  const lastElement = customHeaderElements[customHeaderElements.length - 1]
  lastElement.nextElementSibling.insertAdjacentHTML('afterend', customHeaderElement)
}

function checkPayload () {
  const len = document.getElementById('payload').value.length
  // console.log("payload length is ", len)
  if (len > 100) {
    document.getElementById('run-form').method = 'POST'
  }
}
// same color as darkmode bg color (darker luminance than logo middle)
Chart.defaults.global.defaultFontColor = 'hsl(16, 67%, 7%)';
