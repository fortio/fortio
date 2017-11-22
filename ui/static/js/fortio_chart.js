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

function showChart(title) {
    document.getElementById('running').style.display = 'none';
    var chartEl = document.getElementById('chart1');
    chartEl.style.visibility = 'visible';
    document.getElementById('update').style.visibility = 'visible';
    var ctx = chartEl.getContext('2d');
    chart = new Chart(ctx, {
        type: 'line',
        data: {
            datasets: [{
                    label: 'Cumulative %',
                    data: dataP,
                    fill: false,
                    yAxisID: 'P',
                    stepped: true,
                    backgroundColor: 'rgba(134, 87, 167, 1)',
                    borderColor: 'rgba(134, 87, 167, 1)',
                },
                {
                    label: 'Histogram: Count',
                    data: dataH,
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
                text: title,
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
}

function updateChart() {
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
    chart.update()
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
    chart.update()
}
