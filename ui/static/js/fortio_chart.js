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

var linearXAxe = {
    type: 'linear',
    scaleLabel: {
        display: true,
        labelString: 'Response time in ms',
        ticks: {
            min: 0,
            beginAtZero: true,
        },
    }
}
var logXAxe = {
    type: 'logarithmic',
    scaleLabel: {
        display: true,
        labelString: 'Response time in ms (log scale)'
    },
    ticks: {
        //min: dataH[0].x, // newer chart.js are ok with 0 on x axis too
        callback: function(tick, index, ticks) {
            return tick.toLocaleString()
        }
    }
}
var linearYAxe = {
    id: 'H',
    type: 'linear',
    ticks: {
        beginAtZero: true,
    },
    scaleLabel: {
        display: true,
        labelString: 'Count'
    }
};
var logYAxe = {
    id: 'H',
    type: 'logarithmic',
    display: true,
    ticks: {
        // min: 1, // log mode works even with 0s
        // Needed to not get scientific notation display:
        callback: function(tick, index, ticks) {
            return tick.toString()
        }
    },
    scaleLabel: {
        display: true,
        labelString: 'Count (log scale)'
    }
}

var chart

function myRound(v, digits = 6) {
    p = Math.pow(10, digits)
    return Math.round(v * p) / p
}

function pad(n) {
    return (n < 10) ? ("0" + n) : n;
}

function formatDate(dStr) {
    var d = new Date(dStr)
    return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) + " " +
        pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
}

function makeTitle(res) {
    var title = []
    if (res.Labels != "") {
        title.push(res.Labels + " - " + res.URL + " - " + formatDate(res.StartTime))
    }
    percStr = "min " + myRound(1000. * res.DurationHistogram.Min, 3) + " ms, average " + myRound(1000. * res.DurationHistogram.Avg, 3) + " ms"
    for (var i = 0; i < res.DurationHistogram.Percentiles.length; i++) {
        var p = res.DurationHistogram.Percentiles[i]
        percStr += ", p" + p.Percentile + " " + myRound(1000 * p.Value, 2) + " ms"
    }
    percStr += ", max " + myRound(1000. * res.DurationHistogram.Max, 3) + " ms"
    title.push('Response time histogram at ' + res.RequestedQPS + ' target qps (' +
        myRound(res.ActualQPS, 1) + ' actual) ' + res.NumThreads + ' connections for ' +
        res.RequestedDuration + ' (actual ' + myRound(res.ActualDuration / 1e9, 1) + 's)')
    title.push(percStr)
    return title
}

function fortioResultToJsChartData(res) {
    var dataP = [{
        x: 0.0,
        y: 0.0
    }]
    var len = res.DurationHistogram.Data.length
    for (var i = 0; i < len; i++) {
        var it = res.DurationHistogram.Data[i]
        var x = 0.0
        if (i == 0) {
            // Extra point, 1/N at min itself
            x = 1000. * it.Start
            // nolint: errcheck
            dataP.push({
                x: myRound(x),
                y: myRound(100. / res.DurationHistogram.Count, 3)
            })
        }
        if (i == len - 1) {
            //last point we use the end part (max)
            x = 1000. * it.End
        } else {
            x = 1000. * (it.Start + it.End) / 2.
        }
        dataP.push({
            x: myRound(x),
            y: myRound(it.Percent, 3)
        })
    }
    var dataH = []
    var prev = 1000. * res.DurationHistogram.Data[0].Start
    for (var i = 0; i < len; i++) {
        var it = res.DurationHistogram.Data[i]
        var startX = 1000. * it.Start
        var endX = 1000. * it.End
        if (startX != prev) {
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

function showChart(data) {
    toggleVisibility()
    makeChart(data)
}

function toggleVisibility() {
    document.getElementById('running').style.display = 'none';
    document.getElementById('update').style.visibility = 'visible';
}

function makeChart(data) {
    var chartEl = document.getElementById('chart1');
    chartEl.style.visibility = 'visible';
    var ctx = chartEl.getContext('2d');
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
                },
                {
                    label: 'Histogram: Count',
                    data: data.dataH,
                    yAxisID: 'H',
                    pointStyle: 'rect',
                    radius: 1,
                    borderColor: 'rgba(87, 167, 134, .9)',
                    backgroundColor: 'rgba(87, 167, 134, .75)'
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            title: {
                display: true,
                fontStyle: 'normal',
                text: data.title,
            },
            elements: {
                line: {
                    tension: 0, // disables bezier curves
                }
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
    updateChart() // TODO: should be able to set vs update options
}

function setChartOptions() {
    var form = document.getElementById('updtForm')
    var formMin = form.xmin.value.trim()
    var formMax = form.xmax.value.trim()
    var scales = chart.config.options.scales
    var newAxis
    var newXMin = parseFloat(formMin)
    if (form.xlog.checked) {
        newXAxis = logXAxe
        //if (formMin == "0") {
        //	newXMin = dataH[0].x // log doesn't like 0 xaxis
        //}
    } else {
        newXAxis = linearXAxe
    }
    if (form.ylog.checked) {
        chart.config.options.scales = {
            xAxes: [newXAxis],
            yAxes: [scales.yAxes[0], logYAxe]
        }
    } else {
        chart.config.options.scales = {
            xAxes: [newXAxis],
            yAxes: [scales.yAxes[0], linearYAxe]
        }
    }
    chart.update() // needed for scales.xAxes[0] to exist
    var newNewXAxis = chart.config.options.scales.xAxes[0]
    if (formMin != "") {
        newNewXAxis.ticks.min = newXMin
    } else {
        delete newNewXAxis.ticks.min
    }
    if (formMax != "" && formMax != "max") {
        newNewXAxis.ticks.max = parseFloat(formMax)
    } else {
        delete newNewXAxis.ticks.max
    }
}

function updateChart() {
    setChartOptions()
    chart.update()
}
